package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Fyne widgets keep the arrow cursor; these wrappers show the hand cursor like links on the web.

type button struct{ widget.Button }

func newButton(label string, tapped func()) *button {
	return newIconButton(label, nil, tapped)
}

func newIconButton(label string, icon fyne.Resource, tapped func()) *button {
	b := &button{}
	b.Text, b.Icon, b.OnTapped = label, icon, tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *button) Cursor() desktop.Cursor {
	if b.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

type selectBox struct{ widget.Select }

func newSelect(options []string, changed func(string)) *selectBox {
	s := &selectBox{}
	s.Options, s.OnChanged = options, changed
	s.ExtendBaseWidget(s)
	return s
}

func (s *selectBox) Cursor() desktop.Cursor { return desktop.PointerCursor }

type checkBox struct{ widget.Check }

func newCheck(label string, changed func(bool)) *checkBox {
	c := &checkBox{}
	c.Text, c.OnChanged = label, changed
	c.ExtendBaseWidget(c)
	return c
}

func (c *checkBox) Cursor() desktop.Cursor {
	if c.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}
