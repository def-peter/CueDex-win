// Package speech speaks a fixed phrase with a Windows SAPI voice. The phrase
// never carries anything from an event, a session or a reply: only the
// producer's name, which selects one of two constants.
//
// SAPI is COM, so the voice object lives on one dedicated OS thread and every
// call reaches it through a channel. Cueing therefore never waits for a voice
// to load or for a phrase to end.
package speech

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Voice is one installed SAPI voice.
type Voice struct {
	// ID is the token id SAPI uses to choose the voice again.
	ID string
	// Name is what the user sees, such as "Microsoft Huihui Desktop".
	Name string
	// Language is the voice's language tag, such as "zh-CN".
	Language string
}

// Label is what a picker shows for the voice.
func (v Voice) Label() string {
	if v.Language == "" {
		return v.Name
	}
	return v.Name + "（" + v.Language + "）"
}

// ErrNoVoices means SAPI is present but has no installed voice. The caller
// hides the speech options and keeps cueing with sound.
var ErrNoVoices = errors.New("speech: no SAPI voice is installed")

// ErrUnavailable means SAPI could not be reached at all.
var ErrUnavailable = errors.New("speech: SAPI is unavailable")

// Engine owns the SAPI voice on its own thread. The zero value is not usable;
// call New.
type Engine struct {
	start sync.Once

	mu     sync.Mutex
	cmds   chan request
	closed bool
	failed error
	done   chan struct{}
}

type requestKind int

const (
	requestList requestKind = iota
	requestSpeak
	requestStop
	requestClose
)

type request struct {
	kind    requestKind
	voiceID string
	text    string
	reply   chan listReply
}

type listReply struct {
	voices []Voice
	err    error
}

// New returns an engine. Nothing is loaded until the first call.
func New() *Engine {
	return &Engine{
		cmds: make(chan request, 8),
		done: make(chan struct{}),
	}
}

// Voices lists the installed voices. It is safe from any goroutine, so the
// panel can ask in the background and never wait on SAPI while drawing.
func (e *Engine) Voices() ([]Voice, error) {
	if e == nil {
		return nil, ErrUnavailable
	}
	e.begin()
	reply := make(chan listReply, 1)
	e.mu.Lock()
	closed := e.closed
	failed := e.failed
	e.mu.Unlock()
	if closed {
		return nil, ErrUnavailable
	}
	if failed != nil {
		return nil, failed
	}
	select {
	case e.cmds <- request{kind: requestList, reply: reply}:
	case <-e.done:
		return nil, e.failure()
	}
	select {
	case r := <-reply:
		return r.voices, r.err
	case <-e.done:
		return nil, e.failure()
	}
}

// Speak asks for a phrase. It returns at once: the dedicated thread does the
// work, and a request that arrives while the queue is full is dropped rather
// than making the caller wait. voiceID may be empty for the system default.
func (e *Engine) Speak(voiceID, text string) error {
	if e == nil {
		return ErrUnavailable
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	e.begin()
	e.mu.Lock()
	closed := e.closed
	failed := e.failed
	e.mu.Unlock()
	if closed {
		return ErrUnavailable
	}
	if failed != nil {
		return failed
	}
	select {
	case e.cmds <- request{kind: requestSpeak, voiceID: voiceID, text: text}:
		return nil
	default:
		// A cue that arrives while the last one is still speaking is not
		// worth blocking the UI for; the newest cue already has its own
		// glow and sound.
		return nil
	}
}

// Stop silences the phrase that is speaking, as when the user pauses.
func (e *Engine) Stop() {
	if e == nil {
		return
	}
	e.begin()
	e.mu.Lock()
	done := e.closed || e.failed != nil
	e.mu.Unlock()
	if done {
		return
	}
	select {
	case e.cmds <- request{kind: requestStop}:
	default:
	}
}

// Close stops the engine and gives the thread back. It waits briefly, so a
// quit is never held up by a stuck voice.
func (e *Engine) Close() {
	if e == nil {
		return
	}
	e.begin()
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()
	select {
	case e.cmds <- request{kind: requestClose}:
	default:
	}
	select {
	case <-e.done:
	case <-time.After(500 * time.Millisecond):
	}
}

func (e *Engine) begin() {
	e.start.Do(func() {
		go e.run()
	})
}

// run is the voice's thread: COM is initialized here and every SAPI call
// happens here.
func (e *Engine) run() {
	defer close(e.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInitialize(); err != nil {
		e.setFailed(err)
		return
	}
	defer coUninitialize()

	voices, tokens, err := enumerateVoices()
	if err != nil {
		e.setFailed(err)
		return
	}
	defer releaseTokens(tokens)

	var voice uintptr
	var voiceErr error
	voiceFor := func(id string) uintptr {
		token, ok := tokens[id]
		if !ok {
			return 0
		}
		return token
	}
	// The default voice is whatever SAPI picks; it is loaded on the first
	// phrase so an app that never speaks never pays for a TTS engine.
	ensureVoice := func() error {
		if voice != 0 || voiceErr != nil {
			return voiceErr
		}
		voice, voiceErr = newVoice()
		return voiceErr
	}

	for req := range e.cmds {
		switch req.kind {
		case requestList:
			req.reply <- listReply{voices: append([]Voice(nil), voices...)}
		case requestSpeak:
			if err := ensureVoice(); err != nil {
				// The app keeps cueing with sound; the reason is logged once
				// by the caller's first attempt.
				_ = err
				continue
			}
			if token := voiceFor(req.voiceID); token != 0 {
				call(voice, ispVoiceSetVoice, token)
			}
			call(voice, ispVoiceSetVolume, 100)
			text, err := syscall.UTF16PtrFromString(req.text)
			if err != nil {
				continue
			}
			// Async: the phrase plays while this thread stays free to
			// silence it when the user pauses.
			call(voice, ispVoiceSpeak, uintptr(unsafe.Pointer(text)), spfAsync, 0)
		case requestStop:
			if voice != 0 {
				// A null phrase with this flag cancels what is speaking.
				call(voice, ispVoiceSpeak, 0, spfPurgeBeforeSpeak, 0)
			}
		case requestClose:
			if voice != 0 {
				call(voice, ispVoiceSpeak, 0, spfPurgeBeforeSpeak, 0)
				release(voice)
			}
			return
		}
	}
}

func (e *Engine) setFailed(err error) {
	e.mu.Lock()
	e.failed = err
	e.mu.Unlock()
}

func (e *Engine) failure() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return e.failed
	}
	return ErrUnavailable
}

// GUID as Windows lays it out.
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidSpVoice = guid{0x96749377, 0x3391, 0x11d2, [8]byte{0x9e, 0xe3, 0x00, 0xc0, 0x4f, 0x79, 0x73, 0x96}}
	iidISpVoice  = guid{0x6c44df74, 0x72b9, 0x4992, [8]byte{0xa1, 0xec, 0xef, 0x99, 0x6e, 0x04, 0x22, 0xd4}}

	clsidSpObjectTokenCategory = guid{0xa910187f, 0x0c7a, 0x45ac, [8]byte{0x92, 0xcc, 0x59, 0xed, 0xaf, 0xb7, 0x7b, 0x53}}
	iidISpObjectTokenCategory  = guid{0x2d3d3845, 0x39af, 0x4850, [8]byte{0xbb, 0xf9, 0x40, 0xb4, 0x97, 0x80, 0x01, 0x1d}}
)

const (
	// ISpVoice vtable indices, after IUnknown, ISpNotifySource and
	// ISpEventSource.
	ispVoiceSetVoice  = 18
	ispVoiceSpeak     = 20
	ispVoiceSetVolume = 30

	// ISpObjectTokenCategory: SetId and EnumTokens.
	ispCategorySetID      = 15
	ispCategoryEnumTokens = 18

	// ISpObjectToken: GetId.
	ispTokenGetID = 16

	// ISpDataKey: GetStringValue and OpenKey.
	ispDataKeyGetStringValue = 6
	ispDataKeyOpenKey        = 9

	// IEnumSpObjectTokens: Item and GetCount.
	ispEnumItem     = 7
	ispEnumGetCount = 8

	spfAsync            = 0x1
	spfPurgeBeforeSpeak = 0x2

	clsctxAll = 0x17

	localeSISO639LangName  = 0x59
	localeSISO3166CtryName = 0x5a

	voicesCategory = `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Speech\Voices`
)

var (
	ole32    = syscall.NewLazyDLL("ole32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procGetLocaleInfoW   = kernel32.NewProc("GetLocaleInfoW")
)

func coInitialize() error {
	hr, _, _ := procCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
	if hr != 0 && hr != 1 {                   // S_OK or S_FALSE
		return fmt.Errorf("%w: CoInitializeEx 0x%08x", ErrUnavailable, hr)
	}
	return nil
}

func coUninitialize() { procCoUninitialize.Call() }

func coCreate(clsid *guid, iid *guid) (uintptr, error) {
	var obj uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&obj)),
	)
	if hr != 0 || obj == 0 {
		return 0, fmt.Errorf("%w: CoCreateInstance 0x%08x", ErrUnavailable, hr)
	}
	return obj, nil
}

func newVoice() (uintptr, error) {
	return coCreate(&clsidSpVoice, &iidISpVoice)
}

// call invokes method index of a COM object.
func call(obj uintptr, index int, args ...uintptr) uintptr {
	// COM hands out plain addresses, not Go pointers, so the address is
	// reinterpreted rather than converted: vet rightly distrusts turning an
	// arbitrary uintptr back into unsafe.Pointer.
	object := *(*unsafe.Pointer)(unsafe.Pointer(&obj))
	vtbl := *(*unsafe.Pointer)(object)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(index)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	hr, _, _ := syscall.SyscallN(fn, all...)
	return hr
}

func release(obj uintptr) {
	if obj != 0 {
		call(obj, 2)
	}
}

func enumerateVoices() ([]Voice, map[string]uintptr, error) {
	category, err := coCreate(&clsidSpObjectTokenCategory, &iidISpObjectTokenCategory)
	if err != nil {
		return nil, nil, err
	}
	defer release(category)

	path, err := syscall.UTF16PtrFromString(voicesCategory)
	if err != nil {
		return nil, nil, err
	}
	if hr := call(category, ispCategorySetID, uintptr(unsafe.Pointer(path)), 0); hr != 0 {
		return nil, nil, fmt.Errorf("%w: SetId 0x%08x", ErrUnavailable, hr)
	}
	var enum uintptr
	if hr := call(category, ispCategoryEnumTokens, 0, 0, uintptr(unsafe.Pointer(&enum))); hr != 0 || enum == 0 {
		return nil, nil, fmt.Errorf("%w: EnumTokens 0x%08x", ErrUnavailable, hr)
	}
	defer release(enum)

	var count uint32
	if hr := call(enum, ispEnumGetCount, uintptr(unsafe.Pointer(&count))); hr != 0 {
		return nil, nil, fmt.Errorf("%w: GetCount 0x%08x", ErrUnavailable, hr)
	}

	voices := make([]Voice, 0, count)
	tokens := make(map[string]uintptr, count)
	for i := uint32(0); i < count; i++ {
		var token uintptr
		if hr := call(enum, ispEnumItem, uintptr(i), uintptr(unsafe.Pointer(&token))); hr != 0 || token == 0 {
			continue
		}
		id, err := tokenString(token, ispTokenGetID)
		if err != nil {
			release(token)
			continue
		}
		name, _ := attributeString(token, "Name")
		if name == "" {
			name = id
		}
		language := ""
		if raw, _ := attributeString(token, "Language"); raw != "" {
			language = languageTag(raw)
		}
		voices = append(voices, Voice{ID: id, Name: name, Language: language})
		tokens[id] = token
	}
	if len(voices) == 0 {
		return nil, tokens, ErrNoVoices
	}
	return voices, tokens, nil
}

func releaseTokens(tokens map[string]uintptr) {
	for _, token := range tokens {
		release(token)
	}
}

// tokenString reads a string the token allocates, and frees it.
func tokenString(token uintptr, method int) (string, error) {
	var buf *uint16
	if hr := call(token, method, uintptr(unsafe.Pointer(&buf))); hr != 0 || buf == nil {
		return "", fmt.Errorf("speech: token string 0x%08x", hr)
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(buf)))
	return utf16String(buf), nil
}

// attributeString reads one value from the token's Attributes key.
func attributeString(token uintptr, name string) (string, error) {
	keyName, err := syscall.UTF16PtrFromString("Attributes")
	if err != nil {
		return "", err
	}
	var attributes uintptr
	if hr := call(token, ispDataKeyOpenKey, uintptr(unsafe.Pointer(keyName)), uintptr(unsafe.Pointer(&attributes))); hr != 0 || attributes == 0 {
		return "", fmt.Errorf("speech: attributes 0x%08x", hr)
	}
	defer release(attributes)

	valueName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", err
	}
	var buf *uint16
	if hr := call(attributes, ispDataKeyGetStringValue, uintptr(unsafe.Pointer(valueName)), uintptr(unsafe.Pointer(&buf))); hr != 0 || buf == nil {
		return "", fmt.Errorf("speech: attribute %s 0x%08x", name, hr)
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(buf)))
	return utf16String(buf), nil
}

// utf16String reads a COM string without assuming how long it is.
func utf16String(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for n < 4096 {
		if *(*uint16)(unsafe.Add(unsafe.Pointer(p), uintptr(n)*2)) == 0 {
			break
		}
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

// languageTag turns SAPI's hexadecimal LCID, such as 804, into "zh-CN".
func languageTag(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var lcid uint32
	for _, r := range trimmed {
		var digit uint32
		switch {
		case r >= '0' && r <= '9':
			digit = uint32(r - '0')
		case r >= 'a' && r <= 'f':
			digit = uint32(r-'a') + 10
		case r >= 'A' && r <= 'F':
			digit = uint32(r-'A') + 10
		default:
			return raw
		}
		lcid = lcid*16 + digit
	}
	if lcid == 0 {
		// LCID zero means "the user's locale", not a language of its own.
		return trimmed
	}
	lang := localeName(lcid, localeSISO639LangName)
	country := localeName(lcid, localeSISO3166CtryName)
	if lang == "" {
		return fmt.Sprintf("0x%04X", lcid)
	}
	if country == "" {
		return lang
	}
	return lang + "-" + country
}

func localeName(lcid uint32, kind uintptr) string {
	var buf [32]uint16
	n, _, _ := procGetLocaleInfoW.Call(uintptr(lcid), kind, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n-1])
}
