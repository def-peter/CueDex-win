package app

import "fmt"

// problemKind names a failure the panel can word in either language. The
// detail is the underlying error, kept verbatim because only the system knows
// what went wrong.
type problemKind int

const (
	noProblem problemKind = iota
	overlayProblem
	watchProblem
	soundProblem
	speechProblem
	saveProblem
	hooksProblem
	codexProblemKind
	openCodeMissing
	openCodeUnsupported
	openCodeForeign
	openCodeOther
	autostartProblem
	busyProblem
)

// problem is one failure, or the zero value for none.
type problem struct {
	kind   problemKind
	detail string
}

func newProblem(kind problemKind, detail string) problem {
	return problem{kind: kind, detail: detail}
}

// empty reports whether there is nothing to show.
func (p problem) empty() bool { return p.kind == noProblem }

// text is the sentence the panel shows.
func (p problem) text(t wording) string {
	switch p.kind {
	case overlayProblem:
		return fmt.Sprintf(t.ProblemOverlay, p.detail)
	case watchProblem:
		return fmt.Sprintf(t.ProblemWatch, p.detail)
	case soundProblem:
		return fmt.Sprintf(t.ProblemSound, p.detail)
	case speechProblem:
		return fmt.Sprintf(t.ProblemSpeech, p.detail)
	case saveProblem:
		return fmt.Sprintf(t.ProblemSave, p.detail)
	case hooksProblem:
		return t.ProblemHooks
	case codexProblemKind:
		return fmt.Sprintf(t.ProblemCodex, p.detail)
	case openCodeMissing:
		return t.ProblemOpenNo
	case openCodeUnsupported:
		if p.detail == "" {
			return t.ProblemOpenOldNoVersion
		}
		return fmt.Sprintf(t.ProblemOpenOld, p.detail)
	case openCodeForeign:
		return t.ProblemOpenForeign
	case openCodeOther:
		return fmt.Sprintf(t.ProblemOpenOther, p.detail)
	case autostartProblem:
		return fmt.Sprintf(t.ProblemAutostart, p.detail)
	case busyProblem:
		return fmt.Sprintf(t.ProblemBusy, p.detail)
	}
	return ""
}
