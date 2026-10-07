package ui

import (
	"errors"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// securityForm picks the cipher, the key derivation function and its cost: a calibrated preset,
// custom parameters or (when re-keying) the current setup.
type securityForm struct {
	current  *vault.Security
	cipher   *selectBox
	kdf      *selectBox
	level    *selectBox
	iter     *widget.Entry
	mem      *widget.Entry
	threads  *widget.Entry
	logN     *widget.Entry
	blockR   *widget.Entry
	items    map[string]*widget.FormItem
	form     *widget.Form
	estimate *widget.Label
	obj      fyne.CanvasObject
	onLayout func()
}

var cipherKeys = map[vault.CipherKind]string{
	vault.CipherXChaCha20: "cipher.xchacha",
	vault.CipherAESGCM:    "cipher.aes",
	vault.CipherCascade:   "cipher.cascade",
}

func newSecurityForm(current *vault.Security) *securityForm {
	T := i18n.T
	f := &securityForm{current: current, items: map[string]*widget.FormItem{}}

	var cipherNames []string
	for _, c := range vault.Ciphers {
		cipherNames = append(cipherNames, T(cipherKeys[c]))
	}
	f.cipher = newSelect(cipherNames, nil)
	var kdfNames []string
	for _, k := range vault.KDFs {
		kdfNames = append(kdfNames, k.String())
	}
	f.kdf = newSelect(kdfNames, nil)

	var levels []string
	if current != nil {
		levels = append(levels, T("sec.keep"))
	}
	for _, l := range vault.Levels {
		levels = append(levels, levelLabel(l))
	}
	levels = append(levels, T("sec.custom"))
	f.level = newSelect(levels, nil)

	numEntry := func() *widget.Entry {
		e := widget.NewEntry()
		e.Validator = func(s string) error {
			if _, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32); err != nil {
				return errors.New(T("sec.invalid"))
			}
			return nil
		}
		return e
	}
	f.iter, f.mem, f.threads, f.logN, f.blockR = numEntry(), numEntry(), numEntry(), numEntry(), numEntry()

	f.estimate = widget.NewLabel("")
	f.estimate.Wrapping = fyne.TextWrapWord
	f.estimate.SizeName = theme.SizeNameCaptionText
	test := newIconButton(T("sec.test"), theme.MediaPlayIcon(), f.runTest)

	f.items["iter"] = widget.NewFormItem(T("sec.iterations"), f.iter)
	f.items["mem"] = widget.NewFormItem(T("sec.memory"), f.mem)
	f.items["threads"] = widget.NewFormItem(T("sec.parallelism"), f.threads)
	f.items["logN"] = widget.NewFormItem(T("sec.cost"), f.logN)
	f.items["blockR"] = widget.NewFormItem(T("sec.blocksize"), f.blockR)
	f.form = widget.NewForm(
		widget.NewFormItem(T("sec.cipher"), f.cipher),
		widget.NewFormItem(T("sec.kdf"), f.kdf),
		widget.NewFormItem(T("kdf.level"), f.level),
	)

	start := vault.Security{KDF: vault.DefaultKDF, Cipher: vault.CipherXChaCha20}
	if current != nil {
		start = *current
	}
	f.cipher.SetSelectedIndex(max(0, indexOf(vault.Ciphers, start.Cipher)))
	f.kdf.SetSelectedIndex(max(0, indexOf(vault.KDFs, start.KDF.Kind)))
	f.fillFields(start.KDF)
	if current != nil {
		f.level.SetSelectedIndex(0)
	} else {
		f.level.SetSelectedIndex(vault.DefaultLevel)
	}
	f.kdf.OnChanged = func(string) { f.fillFields(defaultParams(f.kind())); f.layout() }
	f.level.OnChanged = func(string) { f.layout() }
	f.layout()

	f.obj = container.NewVBox(f.form, container.NewBorder(nil, nil, nil, test, f.estimate), labelWrap(T("sec.hint")))
	return f
}

func indexOf[T comparable](list []T, v T) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

func (f *securityForm) kind() vault.KDFKind { return vault.KDFs[max(0, f.kdf.SelectedIndex())] }

func (f *securityForm) isCustom() bool { return f.level.SelectedIndex() == len(f.level.Options)-1 }

func (f *securityForm) isKeep() bool { return f.current != nil && f.level.SelectedIndex() == 0 }

func defaultParams(k vault.KDFKind) vault.KDFParams {
	switch k {
	case vault.KDFScrypt:
		return vault.KDFParams{Kind: k, LogN: 17, BlockSize: 8, Threads: 1}
	case vault.KDFPBKDF2:
		return vault.KDFParams{Kind: k, Iterations: 600000}
	}
	return vault.DefaultKDF
}

func (f *securityForm) fillFields(p vault.KDFParams) {
	if p.Kind != f.kind() {
		p = defaultParams(f.kind())
	}
	set := func(e *widget.Entry, v uint64) { e.SetText(strconv.FormatUint(v, 10)) }
	set(f.iter, uint64(p.Iterations))
	set(f.mem, uint64(p.MemoryKiB/1024))
	set(f.threads, uint64(max(p.Threads, 1)))
	set(f.logN, uint64(p.LogN))
	set(f.blockR, uint64(p.BlockSize))
}

// layout shows the parameter fields that apply to the chosen function in custom mode.
func (f *securityForm) layout() {
	base := f.form.Items[:3]
	items := append([]*widget.FormItem(nil), base...)
	if f.isCustom() {
		switch f.kind() {
		case vault.KDFArgon2id:
			items = append(items, f.items["iter"], f.items["mem"], f.items["threads"])
		case vault.KDFScrypt:
			items = append(items, f.items["logN"], f.items["blockR"], f.items["threads"])
		case vault.KDFPBKDF2:
			items = append(items, f.items["iter"])
		}
	}
	keep := f.isKeep()
	if keep {
		f.cipher.Disable()
		f.kdf.Disable()
	} else {
		f.cipher.Enable()
		f.kdf.Enable()
	}
	f.form.Items = items
	f.form.Refresh()
	f.estimate.SetText("")
	f.changed()
}

func (f *securityForm) changed() {
	if f.onLayout != nil {
		f.onLayout()
	}
}

func parseU(e *widget.Entry) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(e.Text), 10, 32)
	return v
}

// customParams reads the manual fields.
func (f *securityForm) customParams() (vault.KDFParams, error) {
	p := vault.KDFParams{Kind: f.kind()}
	switch p.Kind {
	case vault.KDFArgon2id:
		p.Iterations, p.MemoryKiB, p.Threads = uint32(parseU(f.iter)), uint32(parseU(f.mem)*1024), uint8(min(parseU(f.threads), 255))
	case vault.KDFScrypt:
		p.LogN, p.BlockSize, p.Threads = uint8(min(parseU(f.logN), 255)), uint32(parseU(f.blockR)), uint8(min(parseU(f.threads), 255))
	case vault.KDFPBKDF2:
		p.Iterations = uint32(parseU(f.iter))
	}
	if err := vault.ValidateKDF(p); err != nil {
		return p, errors.New(i18n.T("sec.invalid") + ": " + err.Error())
	}
	return p, nil
}

// check validates the inputs on the UI thread before any slow work starts.
func (f *securityForm) check() error {
	if f.isCustom() {
		_, err := f.customParams()
		return err
	}
	return nil
}

// resolver captures the choice on the UI thread; the returned func may run off it because
// calibration takes a moment.
func (f *securityForm) resolver() func() (vault.Security, error) {
	if f.isKeep() {
		cur := *f.current
		return func() (vault.Security, error) { return cur, nil }
	}
	cipher := vault.Ciphers[max(0, f.cipher.SelectedIndex())]
	if f.isCustom() {
		p, err := f.customParams()
		return func() (vault.Security, error) { return vault.Security{KDF: p, Cipher: cipher}, err }
	}
	kind := f.kind()
	off := 0
	if f.current != nil {
		off = 1
	}
	lvl := vault.Levels[max(0, f.level.SelectedIndex()-off)]
	return func() (vault.Security, error) {
		return vault.Security{KDF: vault.Calibrate(kind, lvl), Cipher: cipher}, nil
	}
}

func (f *securityForm) runTest() {
	T := i18n.T
	if err := f.check(); err != nil {
		f.estimate.SetText(err.Error())
		return
	}
	resolve := f.resolver()
	f.estimate.SetText(T("sec.testing"))
	go func() {
		sec, err := resolve()
		msg := ""
		if err != nil {
			msg = err.Error()
		} else {
			d := sec.KDF.Benchmark()
			msg = T("sec.estimate", d.Seconds(), sec.KDF.MemoryMiB()) + "\n" + securitySummary(sec, false)
		}
		fyne.Do(func() { f.estimate.SetText(msg); f.changed() })
	}()
}
