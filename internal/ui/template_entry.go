package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/TheGeeKing/FileGot/internal/media"
)

type templateAssistPopup struct {
	widget.BaseWidget
	content fyne.CanvasObject
	entry   *advancedTemplateEntry
}

func newTemplateAssistPopup(content fyne.CanvasObject, entry *advancedTemplateEntry) *templateAssistPopup {
	popup := &templateAssistPopup{content: content, entry: entry}
	popup.ExtendBaseWidget(popup)
	return popup
}

func (popup *templateAssistPopup) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(popup.content)
}

func (popup *templateAssistPopup) FocusGained() {}

func (popup *templateAssistPopup) FocusLost() {}

func (popup *templateAssistPopup) TypedRune(character rune) {
	popup.entry.TypedRune(character)
}

func (popup *templateAssistPopup) TypedKey(key *fyne.KeyEvent) {
	popup.entry.TypedKey(key)
}

type advancedTemplateEntry struct {
	widget.Entry

	kind              media.Kind
	enabled           func() bool
	onChanged         func(string)
	completions       []media.AdvancedTemplateCompletion
	selected          int
	signature         *media.AdvancedTemplateSignature
	popup             *widget.PopUp
	completionScroll  *container.Scroll
	completionButtons []*widget.Button
	signatureLabel    *widget.Label
	dismissed         bool
	accepting         bool
}

func newAdvancedTemplateEntry(kind media.Kind, enabled func() bool) *advancedTemplateEntry {
	entry := &advancedTemplateEntry{
		kind:    kind,
		enabled: enabled,
	}
	entry.ExtendBaseWidget(entry)
	entry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	entry.Entry.OnChanged = entry.changed
	entry.Entry.OnCursorChanged = entry.cursorChanged
	return entry
}

func (entry *advancedTemplateEntry) setOnChanged(changed func(string)) {
	entry.onChanged = changed
}

func (entry *advancedTemplateEntry) changed(text string) {
	if entry.onChanged != nil {
		entry.onChanged(text)
	}
	if entry.accepting {
		return
	}
	entry.dismissed = false
	entry.refreshAssist()
}

func (entry *advancedTemplateEntry) cursorChanged() {
	if entry.accepting {
		return
	}
	entry.dismissed = false
	entry.refreshAssist()
}

func (entry *advancedTemplateEntry) TypedRune(character rune) {
	entry.dismissed = false
	entry.Entry.TypedRune(character)
}

func (entry *advancedTemplateEntry) TypedKey(key *fyne.KeyEvent) {
	if len(entry.completions) > 0 {
		switch key.Name {
		case fyne.KeyDown:
			entry.selectCompletion((entry.selected + 1) % len(entry.completions))
			return
		case fyne.KeyUp:
			entry.selectCompletion((entry.selected - 1 + len(entry.completions)) % len(entry.completions))
			return
		case fyne.KeyEnter, fyne.KeyReturn, fyne.KeyTab:
			entry.acceptCompletion(entry.selected)
			return
		case fyne.KeyEscape:
			entry.dismiss()
			return
		}
	} else if entry.signature != nil && key.Name == fyne.KeyEscape {
		entry.dismiss()
		return
	}

	entry.dismissed = false
	entry.Entry.TypedKey(key)
	entry.refreshAssist()
}

func (entry *advancedTemplateEntry) refreshAssist() {
	if entry.enabled != nil && !entry.enabled() || entry.dismissed {
		entry.hidePopup()
		entry.completions = nil
		entry.signature = nil
		return
	}

	cursor := entry.CursorTextOffset()
	entry.completions = media.AdvancedTemplateCompletions(entry.kind, entry.Text, cursor)
	entry.selected = 0
	if len(entry.completions) > 0 {
		entry.signature = nil
		entry.showCompletions()
		return
	}

	entry.signature = media.AdvancedTemplateSignatureHelp(entry.kind, entry.Text, cursor)
	if entry.signature != nil {
		entry.showSignature()
		return
	}
	entry.hidePopup()
}

func (entry *advancedTemplateEntry) showCompletions() {
	entry.hidePopup()
	entry.completionButtons = make([]*widget.Button, len(entry.completions))
	rows := make([]fyne.CanvasObject, len(entry.completions))
	for index := range entry.completions {
		completion := entry.completions[index]
		button := widget.NewButton(
			fmt.Sprintf(
				"%s  → %s\n%s · %s",
				completion.Name,
				completion.ReturnType,
				completion.Description,
				completion.Syntax,
			),
			func() { entry.acceptCompletion(index) },
		)
		button.Alignment = widget.ButtonAlignLeading
		entry.completionButtons[index] = button
		rows[index] = button
	}
	scroll := container.NewVScroll(container.NewVBox(rows...))
	height := float32(min(len(rows)*56, 280))
	scroll.SetMinSize(fyne.NewSize(620, height))
	entry.completionScroll = scroll
	entry.showPopup(scroll, fyne.NewSize(620, height))
	entry.selectCompletion(0)
}

func (entry *advancedTemplateEntry) showSignature() {
	entry.hidePopup()
	signature := entry.signature
	parameters := make([]string, len(signature.Parameters))
	for index, parameter := range signature.Parameters {
		optional := ""
		if !parameter.Required {
			optional = "?"
		}
		parameters[index] = parameter.Name + optional + ": " + parameter.Type.String()
	}
	title := widget.NewLabel(fmt.Sprintf(
		"%s(%s)  → %s",
		signature.Name,
		strings.Join(parameters, ", "),
		signature.ReturnType,
	))
	title.TextStyle = fyne.TextStyle{Bold: true}

	detail := "No parameters."
	if len(signature.Parameters) > 0 {
		parameter := signature.Parameters[signature.ActiveParameter]
		requirement := "optional"
		if parameter.Required {
			requirement = "required"
		}
		detail = fmt.Sprintf(
			"Argument %d · %s · %s · %s\n%s",
			signature.ActiveParameter+1,
			parameter.Name,
			parameter.Type,
			requirement,
			parameter.Description,
		)
	}
	entry.signatureLabel = widget.NewLabel(detail)
	entry.signatureLabel.TextStyle = fyne.TextStyle{Bold: true}
	entry.signatureLabel.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(title, entry.signatureLabel)
	entry.showPopup(content, fyne.NewSize(620, content.MinSize().Height+16))
}

func (entry *advancedTemplateEntry) showPopup(content fyne.CanvasObject, size fyne.Size) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	canvas := app.Driver().CanvasForObject(entry)
	if canvas == nil {
		return
	}
	assist := newTemplateAssistPopup(content, entry)
	entry.popup = widget.NewPopUp(assist, canvas)
	entry.popup.Resize(size)
	entry.popup.ShowAtRelativePosition(entry.CursorPosition().Add(fyne.NewPos(0, 28)), entry)
	canvas.Focus(assist)
}

func (entry *advancedTemplateEntry) selectCompletion(index int) {
	entry.selected = index
	for current, button := range entry.completionButtons {
		if current == index {
			button.Importance = widget.HighImportance
		} else {
			button.Importance = widget.MediumImportance
		}
		button.Refresh()
	}
	entry.scrollCompletionIntoView(index)
}

func (entry *advancedTemplateEntry) scrollCompletionIntoView(index int) {
	if entry.completionScroll == nil || index < 0 || index >= len(entry.completionButtons) {
		return
	}
	button := entry.completionButtons[index]
	top := button.Position().Y
	bottom := top + button.Size().Height
	offset := entry.completionScroll.Offset
	viewportBottom := offset.Y + entry.completionScroll.Size().Height
	switch {
	case top < offset.Y:
		offset.Y = top
	case bottom > viewportBottom:
		offset.Y = bottom - entry.completionScroll.Size().Height
	default:
		return
	}
	entry.completionScroll.ScrollToOffset(offset)
}

func (entry *advancedTemplateEntry) acceptCompletion(index int) {
	if index < 0 || index >= len(entry.completions) {
		return
	}
	completion := entry.completions[index]
	text := []rune(entry.Text)
	if completion.ReplaceStart < 0 || completion.ReplaceEnd > len(text) ||
		completion.ReplaceStart > completion.ReplaceEnd {
		return
	}
	insert := []rune(completion.InsertText)
	updated := make([]rune, 0, len(text)+len(insert)-(completion.ReplaceEnd-completion.ReplaceStart))
	updated = append(updated, text[:completion.ReplaceStart]...)
	updated = append(updated, insert...)
	updated = append(updated, text[completion.ReplaceEnd:]...)
	cursor := completion.ReplaceStart + len(insert) - completion.CursorBack

	entry.accepting = true
	entry.SetText(string(updated))
	entry.CursorRow = 0
	entry.CursorColumn = cursor
	entry.Refresh()
	entry.accepting = false
	entry.dismissed = false
	entry.refreshAssist()
}

func (entry *advancedTemplateEntry) dismiss() {
	entry.dismissed = true
	entry.completions = nil
	entry.signature = nil
	entry.hidePopup()
}

func (entry *advancedTemplateEntry) hidePopup() {
	if entry.popup != nil {
		entry.popup.Hide()
		entry.popup = nil
	}
	entry.completionButtons = nil
	entry.completionScroll = nil
	entry.signatureLabel = nil
}
