package app

import (
	"bytes"
	"image/png"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/def-peter/cuedex-win/internal/recordings"

	_ "embed"
)

// The settings window's own look: three tones
// (page, source rail, card) so navigation, page and controls read as layers,
// one hairline, one accent. The theme is replaced rather than tuned because
// the stock one has a single surface for both the page and a control.
type palette struct {
	theme *ui.Theme
	page  ui.Color
	rail  ui.Color
	card  ui.Color
	hair  ui.Color
	ink   ui.Color
	muted ui.Color
	// faint is the third grey, for what explains a control rather than
	// naming it: hints, footnotes, a group's own label.
	faint  ui.Color
	accent ui.Color
	// deep is the accent dark enough for small text on white, which the
	// bright accent is not: 13px blue on white needs the darker one.
	deep ui.Color
}

// paletteFor returns the panel's look matching the system's appearance. The
// panel keeps its own two palettes because the design was drawn in light;
// a dark Windows gets the same layout in dark tones rather than a light panel.
func paletteFor(base *ui.Theme) palette {
	dark := base != nil && base.Dark
	var p palette
	if dark {
		p = palette{
			page:   ui.Hex("#1B1C1F"),
			rail:   ui.Hex("#222327"),
			card:   ui.Hex("#26272B"),
			hair:   ui.Hex("#34353A"),
			ink:    ui.Hex("#F2F3F5"),
			muted:  ui.Hex("#A2A6AD"),
			faint:  ui.Hex("#7C8087"),
			accent: ui.Hex("#3D94FF"),
			deep:   ui.Hex("#6FB0FF"),
		}
	} else {
		p = palette{
			page:   ui.Hex("#F5F6F8"),
			rail:   ui.Hex("#EFF1F4"),
			card:   ui.Hex("#FFFFFF"),
			hair:   ui.Hex("#E3E5E9"),
			ink:    ui.Hex("#1F2023"),
			muted:  ui.Hex("#5F6368"),
			faint:  ui.Hex("#8A8D94"),
			accent: ui.Hex("#3D94FF"),
			deep:   ui.Hex("#2670D9"),
		}
	}
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	t.Background = p.page
	t.Surface = p.card
	t.SurfaceHover = p.rail
	t.SurfacePressed = p.rail
	t.Border = p.hair
	t.Text = p.ink
	t.TextMuted = p.muted
	t.Accent = p.accent
	t.AccentHover = ui.Hex("#5AA5FF")
	t.AccentPressed = p.deep
	t.AccentText = ui.Hex("#FFFFFF")
	t.Selection = p.accent.Alpha(0.25)
	t.Focus = p.accent.Alpha(0.55)
	t.Radius = 8
	t.Spacing = 4
	t.FontSize = 13
	t.Font = "Microsoft YaHei UI"
	p.theme = t
	return p
}

// pageWidth is the measure every page draws to: 560 DIPs, centred in the 660
// DIP window. A row that runs the full window reads as a table; at this width
// the label and its control stay one glance apart.
const pageWidth = 560

// page is the frame every tab draws into: the light page, one centred column.
func (p palette) pageFrame(c *ui.Context, fn func()) {
	ui.Scroll(c).Grow(1).Background(p.page).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).Width(pageWidth).Padding(22, 0, 18).Children(fn)
		})
	})
}

// pageTitle is the line naming the page.
func (p palette) pageTitle(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(20).FontWeight(600).TextColor(p.ink)
}

// group is one inset block: a white rounded panel whose rows are separated by
// hairlines. It carries a page's structure without a card around every
// subject. The label sits above the panel, on the page; an empty one leaves
// the room for it, so groups line up whether they are named or not.
func (p palette) group(c *ui.Context, label string, fn func()) {
	if label != "" {
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(p.faint).
			Padding(12, 2, 6, 2)
	}
	ui.Column(c).FillWidth().Background(p.card).Border(1, p.hair).Radius(10).
		Clip().Children(fn)
}

// row is one line of a group: its label, an optional line under it, and the
// control at the right edge.
func (p palette) row(c *ui.Context, label, hint string, control func()) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Padding(9, 14).
		Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				if label != "" {
					ui.Text(c, label).FontSize(13).TextColor(p.ink)
				}
				if hint != "" {
					ui.Text(c, hint).FontSize(12).TextColor(p.faint)
				}
			})
			control()
		})
}

// rule separates two rows of one group.
func (p palette) rule(c *ui.Context) {
	ui.Box(c).FillWidth().Height(1).Background(p.hair)
}

// footnote explains a group from under it, as the Mac's settings do, so every
// row stays one line.
func (p palette) footnote(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).TextColor(p.faint).Padding(7, 2, 0, 2)
}

// note is a line of explanation, never a control.
func (p palette) note(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).TextColor(p.muted)
}

// hairline separates the tab bar from the page.
func (p palette) hairline(c *ui.Context) {
	ui.Box(c).FillWidth().Height(1).Background(p.hair)
}

// segmented is a picker of the design's shape: a grey track with the chosen
// segment raised in white. MyGo's own Segmented tints it from the theme, which
// with this palette would put a white track on a white card. onPage says the
// control sits on the page rather than on a card, where the track's own tone
// is too close to the page's and needs its hairline drawn.
func (p palette) segmented(c *ui.Context, selected *int, label string, width float32, onPage bool, change func(), labels ...string) ui.Element {
	track := p.rail
	if onPage {
		track = p.hair
	}
	parts := ui.SegmentedBase(c, selected, len(labels))
	trackEl := parts.Track.Width(width).Background(p.rail).Border(1, track).Radius(8).
		Padding(2).Gap(2).Label(label).OnChange(change)
	trackEl.Children(func() {
		for i, choice := range labels {
			seg := parts.Segment(i).Grow(1).Height(32).Radius(6).Center()
			if i == *selected {
				seg.Background(p.card).Border(1, p.hair)
				seg.Children(func() {
					ui.Text(c, choice).FontSize(13).FontWeight(600).TextColor(p.deep)
				})
			} else {
				seg.Children(func() {
					ui.Text(c, choice).FontSize(13).TextColor(p.muted)
				})
			}
		}
	})
	return trackEl
}

// recordingPicker chooses one of the bundled clips. Its menu groups the clips
// by the language they speak, as the mac app's sections do, and marks the clip
// in use; each heading is a disabled item, so it cannot be chosen.
func (p palette) recordingPicker(c *ui.Context, t wording, value *string, enabled bool, change func()) {
	ui.PopoverMenuButton(c, recordingLabel(*value, t), func(m *ui.Menu) {
		labels := t.recordingLabels()
		last := ""
		for i, clip := range recordings.All() {
			if clip.Language != last {
				if last != "" {
					m.Separator()
				}
				m.Item(t.clipLanguage(clip.Language)).Disabled(true)
				last = clip.Language
			}
			label := clip.ID
			if i < len(labels) {
				label = labels[i]
			}
			if m.Item(label).Checked(clip.ID == *value).Chosen() {
				*value = clip.ID
				change()
			}
		}
	}).Width(240).Height(32).Disabled(!enabled).Label(t.SpeechRecording)
}

//go:embed icon_128x128.png
var icon128 []byte

// aboutIcon decodes the product icon once; the panel draws it at 64 DIPs on
// the About page, where the 32 pixel tray image would blur.
var aboutIcon = sync.OnceValue(func() *ui.Bitmap {
	img, err := png.Decode(bytes.NewReader(icon128))
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img)
})
