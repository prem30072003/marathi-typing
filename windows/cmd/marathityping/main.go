//go:build windows

// Marathi Typing for Windows: type Marathi in English letters in any app
// (MS Word, Excel, PowerPoint, Outlook, Notepad, browsers, WhatsApp desktop)
// and get Devanagari.
//
// How it works: a low-level keyboard hook holds the letters you type in a
// small suggestion window (they don't reach the app yet). When you press
// Space / Enter / a number, the chosen Devanagari word is typed into the
// focused app with SendInput (as Unicode characters), exactly like a
// keyboard would. Alt+M switches between Marathi and English.
//
// The mode is always visible in a small floating म / EN badge (drag it
// anywhere, click to switch, right-click for the menu) and in the tray.
package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"marathityping/engine"
)

var (
	//go:embed lexicon.tsv.gz
	lexiconGz []byte
	//go:embed en-words.txt
	englishTxt []byte
	//go:embed mr-on.ico
	icoOn []byte
	//go:embed mr-off.ico
	icoOff []byte
)

const (
	appName     = "Marathi Typing"
	version     = "0.2.0"
	injectedTag = 0x4D524154 // "MRAT" in dwExtraInfo marks our own SendInput events
	nSuggest    = 5
	nComplete   = 2
	idHotkey    = 1
	idSaveTimer = 1
	idFlash     = 2
	idDictTimer = 3
	idTrayRetry = 4

	cmdToggle  = 100
	cmdStartup = 101
	cmdForget  = 102
	cmdHelp    = 103
	cmdExit    = 104
	cmdEnglish = 105
	cmdDigits  = 106
	cmdBadge   = 107
	cmdDict    = 108
	cmdFolder  = 109
)

const defaultDict = `# My dictionary for Marathi Typing
# One entry per line. Save this file and the app picks up the changes within a few seconds.
#
#   shortcut = what it should type      (any text, even several words)
#   देवनागरी                            (a word of your own that loose spellings should find)
#
# Examples - remove the # to use them:
# gm = शुभ सकाळ!
# addr = १२, शिवाजी नगर, पुणे ४११००५
# कोल्हटकर
`

var (
	eng       *engine.Engine
	hInstance uintptr
	mainHwnd  uintptr
	popHwnd   uintptr
	badgeHwnd uintptr
	kbHook    uintptr
	msHook    uintptr

	buf      []byte // Roman letters typed so far
	cands    []string
	nComp    int // how many of cands (at the end) are completions
	sel      int
	picked   bool
	prevWord string // last word typed, for context

	fontLatin, fontDeva, fontSmall, fontBadge uintptr
	iconOn, iconOff                           uintptr
	dpiScale                                  = 1.0
	dataDir                                   string
	hotkeyByHook                              bool
	statusText                                string
	msgTaskbarCreated                         uint32
	trayAdded                                 bool
	trayTries                                 int
	dictMod                                   time.Time

	cfg = settings{Enabled: true, Badge: true}
)

type settings struct {
	Enabled     bool  `json:"enabled"`
	KeepEnglish bool  `json:"keepEnglish"`
	Digits      bool  `json:"digits"`
	Badge       bool  `json:"badge"`
	BadgeX      int32 `json:"badgeX"`
	BadgeY      int32 `json:"badgeY"`
	Welcomed    bool  `json:"welcomed"`
}

func main() {
	runtime.LockOSThread()

	// one copy only
	if _, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("MarathiTypingSingleInstance")))); e == syscall.ERROR_ALREADY_EXISTS {
		messageBox("Marathi Typing is already running.\n\nLook for the orange म badge on your screen (bottom-right), or the म icon in the taskbar tray.", appName, MB_OK|MB_ICONINFORMATION)
		return
	}

	pSetProcessDPIAware.Call()
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d > 0 {
			dpiScale = float64(d) / 96.0
		}
	}

	dataDir = filepath.Join(os.Getenv("APPDATA"), "MarathiTyping")
	os.MkdirAll(dataDir, 0o755)
	loadSettings()

	eng = engine.New()
	zr, err := gzip.NewReader(bytes.NewReader(lexiconGz))
	if err == nil {
		err = eng.Load(zr)
	}
	if err != nil {
		messageBox("Could not load the Marathi word list: "+err.Error(), appName, MB_OK|MB_ICONWARNING)
	}
	eng.LoadEnglish(bytes.NewReader(englishTxt))
	eng.KeepEnglish = cfg.KeepEnglish
	if b, err := os.ReadFile(filepath.Join(dataDir, "learned.json")); err == nil {
		eng.ImportLearned(b)
	}
	loadDict()

	hInstance, _, _ = pGetModuleHandleW.Call(0)
	createFonts()
	iconOn = loadIcon(icoOn, "icon-on.ico", rgb(194, 65, 12))
	iconOff = loadIcon(icoOff, "icon-off.ico", rgb(120, 113, 108))
	r, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	msgTaskbarCreated = uint32(r)
	createWindows()
	addTray()
	if cfg.Badge {
		showBadge()
	}

	if r, _, _ := pRegisterHotKey.Call(mainHwnd, idHotkey, MOD_ALT|MOD_NOREPEAT, 'M'); r == 0 {
		hotkeyByHook = true // someone else owns Alt+M: catch it in the keyboard hook instead
	}

	kbHook, _, _ = pSetWindowsHookExW.Call(WH_KEYBOARD_LL, syscall.NewCallback(keyboardProc), hInstance, 0)
	msHook, _, _ = pSetWindowsHookExW.Call(WH_MOUSE_LL, syscall.NewCallback(mouseProc), hInstance, 0)
	if kbHook == 0 {
		messageBox("Could not start the keyboard hook.", appName, MB_OK|MB_ICONWARNING)
		return
	}
	pSetTimer.Call(mainHwnd, idDictTimer, 2000, 0)

	if !cfg.Welcomed {
		cfg.Welcomed = true
		saveSettings()
		pPostMessageW.Call(mainHwnd, WM_APP+2, 0, 0) // welcome box once the message loop runs
	} else if cfg.Enabled {
		balloon("मराठी टायपिंग चालू आहे", "Alt+M switches Marathi ⇄ English.")
	}

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	pUnhookWindowsHookEx.Call(kbHook)
	pUnhookWindowsHookEx.Call(msHook)
	saveLearned()
	saveSettings()
	removeTray()
}

const welcomeText = "मराठी टायपिंग is running.\n\n" +
	"• Type Marathi the way you chat:  tumhi kase aahat  →  तुम्ही कसे आहात\n" +
	"• Space types the highlighted word, 1–7 pick another, Esc keeps English.\n" +
	"• Alt+M switches Marathi ⇄ English.\n\n" +
	"Where is it?\n" +
	"• The orange म badge (bottom-right of your screen) shows the mode. Drag it anywhere, click it to switch, right-click it for settings.\n" +
	"• Windows 11 hides new tray icons: click the ^ arrow next to the clock to find म, and drag it onto the taskbar to keep it visible.\n\n" +
	"Right-click the badge → \"Edit my words…\" to add your own words and shortcuts."

// ---------------------------------------------------------------- settings

func loadSettings() {
	if b, err := os.ReadFile(filepath.Join(dataDir, "settings.json")); err == nil {
		json.Unmarshal(b, &cfg)
	}
}

func saveSettings() {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0o644)
}

func saveLearned() {
	if b, err := eng.ExportLearned(); err == nil {
		os.WriteFile(filepath.Join(dataDir, "learned.json"), b, 0o644)
	}
}

func dictPath() string { return filepath.Join(dataDir, "my-words.txt") }

func loadDict() {
	p := dictPath()
	st, err := os.Stat(p)
	if err != nil {
		os.WriteFile(p, []byte(strings.ReplaceAll(defaultDict, "\n", "\r\n")), 0o644)
		st, err = os.Stat(p)
		if err != nil {
			return
		}
	}
	dictMod = st.ModTime()
	if b, err := os.ReadFile(p); err == nil {
		eng.SetUserDict(string(b))
	}
}

func checkDict() {
	if st, err := os.Stat(dictPath()); err == nil && !st.ModTime().Equal(dictMod) {
		loadDict()
		flash("✓ my words")
	}
}

// ------------------------------------------------------------ keyboard hook

func isEnglishLetterVK(vk uint32) bool { return vk >= 'A' && vk <= 'Z' }

func focusIsPassword() bool {
	fg, _, _ := pGetForegroundWindow.Call()
	if fg == 0 {
		return false
	}
	tid, _, _ := pGetWindowThreadProcId.Call(fg, 0)
	var gi GUITHREADINFO
	gi.CbSize = uint32(unsafe.Sizeof(gi))
	if r, _, _ := pGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gi))); r == 0 || gi.HwndFocus == 0 {
		return false
	}
	style, _, _ := pGetWindowLongPtrW.Call(gi.HwndFocus, ^uintptr(15)) // GWL_STYLE = -16
	var cls [64]uint16
	pGetClassNameW.Call(gi.HwndFocus, uintptr(unsafe.Pointer(&cls[0])), 64)
	return style&ES_PASSWORD != 0 && strings.EqualFold(syscall.UTF16ToString(cls[:]), "Edit")
}

func ownForeground() bool {
	fg, _, _ := pGetForegroundWindow.Call()
	var pid uint32
	pGetWindowThreadProcId.Call(fg, uintptr(unsafe.Pointer(&pid)))
	me, _, _ := pGetCurrentProcessId.Call()
	return uintptr(pid) == me
}

func keyboardProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) == 0 {
		k := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		if k.Flags&LLKHF_INJECTED == 0 && (wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN) {
			if handleKey(k) {
				return 1
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

const devaDigits = "०१२३४५६७८९"

func digitOf(vk uint32) int {
	if vk >= '0' && vk <= '9' {
		return int(vk - '0')
	}
	if vk >= VK_NUMPAD0 && vk <= VK_NUMPAD9 {
		return int(vk - VK_NUMPAD0)
	}
	return -1
}

// handleKey returns true when the key is consumed.
func handleKey(k *KBDLLHOOKSTRUCT) bool {
	vk := k.VkCode
	ctrl := keyDown(VK_CONTROL)
	alt := keyDown(VK_MENU)
	win := keyDown(VK_LWIN) || keyDown(VK_RWIN)

	if hotkeyByHook && vk == 'M' && alt && !ctrl && !win {
		if len(buf) > 0 {
			commit("", false, nil)
		}
		toggle()
		return true
	}
	if !cfg.Enabled {
		return false
	}

	// modifiers: finish the pending word first so shortcuts act on committed text
	switch vk {
	case VK_SHIFT, VK_LSHIFT, VK_RSHIFT, VK_CAPITAL:
		return false
	case VK_CONTROL, VK_LCONTROL, VK_RCONTROL, VK_MENU, VK_LMENU, VK_RMENU, VK_LWIN, VK_RWIN:
		if len(buf) > 0 {
			commit("", false, k)
			return true
		}
		return false
	}
	if ctrl || alt || win {
		return false
	}
	shift := keyDown(VK_SHIFT)

	if isEnglishLetterVK(vk) {
		if len(buf) == 0 && (ownForeground() || focusIsPassword()) {
			return false
		}
		caps := func() bool { r, _, _ := pGetKeyState.Call(VK_CAPITAL); return r&1 != 0 }()
		c := byte(vk) // 'A'..'Z'
		if shift == caps {
			c += 'a' - 'A'
		}
		buf = append(buf, c)
		sel, picked = 0, false
		refresh()
		return true
	}
	if len(buf) == 0 {
		switch {
		case vk == VK_RETURN || vk == VK_TAB || vk == VK_ESCAPE || vk == VK_UP || vk == VK_DOWN:
			prevWord = ""
		case cfg.Digits && !shift && digitOf(vk) >= 0 && !ownForeground():
			d := []rune(devaDigits)[digitOf(vk)]
			sendText(string(d), nil)
			prevWord = ""
			return true
		case vk == VK_OEM_PERIOD || (shift && vk == '1') || (shift && vk == VK_OEM_2):
			prevWord = "" // . ! ? end the sentence
		}
		return false
	}

	switch vk {
	case VK_BACK:
		buf = buf[:len(buf)-1]
		sel = 0
		if len(buf) == 0 {
			resetBuf()
		} else {
			refresh()
		}
		return true
	case VK_ESCAPE:
		commit("", true, nil)
		return true
	case VK_SPACE:
		commit(" ", false, nil)
		return true
	case VK_RETURN, VK_TAB:
		commit("", false, nil)
		prevWord = ""
		return true
	case VK_UP, VK_DOWN:
		if n := len(cands); n > 0 {
			if vk == VK_DOWN {
				sel = (sel + 1) % n
			} else {
				sel = (sel + n - 1) % n
			}
			picked = true
			showPopup()
		}
		return true
	}
	if !shift {
		if n := digitOf(vk) - 1; n >= 0 && n < len(cands) {
			sel, picked = n, true
			commit("", false, nil)
			return true
		}
	}
	// any other key (punctuation, digits, arrows, Home, Delete ...):
	// type the word first, then replay the key
	commit("", false, k)
	prevWord = ""
	return true
}

func refresh() {
	cands = eng.Suggest(string(buf), nSuggest, prevWord)
	comp := eng.Complete(string(buf), nComplete, cands)
	nComp = len(comp)
	cands = append(cands, comp...)
	if sel >= len(cands) {
		sel = 0
	}
	showPopup()
}

func resetBuf() {
	buf = buf[:0]
	cands = nil
	nComp = 0
	sel, picked = 0, false
	if statusText == "" {
		pShowWindow.Call(popHwnd, SW_HIDE)
	} else {
		showPopup()
	}
}

// commit types the chosen word (+suffix) and, if replay != nil, re-sends that key after it.
func commit(suffix string, raw bool, replay *KBDLLHOOKSTRUCT) {
	input := string(buf)
	word := input
	if !raw {
		if len(cands) == 0 {
			refreshSilently()
		}
		if sel < len(cands) {
			word = cands[sel]
		} else if len(cands) > 0 {
			word = cands[0]
		}
		if picked && sel > 0 {
			eng.Learn(input, word)
		}
		eng.LearnContext(prevWord, word)
		pSetTimer.Call(mainHwnd, idSaveTimer, 3000, 0)
		prevWord = word
	} else {
		prevWord = ""
	}
	resetBuf()
	sendText(word+suffix, replay)
}

func refreshSilently() {
	cands = eng.Suggest(string(buf), nSuggest, prevWord)
}

func sendText(s string, replay *KBDLLHOOKSTRUCT) {
	units, _ := syscall.UTF16FromString(s)
	units = units[:len(units)-1]
	inputs := make([]kbInput, 0, len(units)*2+1)
	for _, u := range units {
		for _, up := range []uint32{0, KEYEVENTF_KEYUP} {
			inputs = append(inputs, kbInput{Type: INPUT_KEYBOARD, Ki: keybdInput{WScan: u, DwFlags: KEYEVENTF_UNICODE | up, DwExtraInfo: injectedTag}})
		}
	}
	if replay != nil {
		fl := uint32(0)
		if replay.Flags&1 != 0 { // LLKHF_EXTENDED
			fl |= KEYEVENTF_EXTENDEDKEY
		}
		inputs = append(inputs, kbInput{Type: INPUT_KEYBOARD, Ki: keybdInput{WVk: uint16(replay.VkCode), WScan: uint16(replay.ScanCode), DwFlags: fl, DwExtraInfo: injectedTag}})
	}
	if len(inputs) > 0 {
		pSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	}
}

// -------------------------------------------------------------- mouse hook

// A click while a word is pending: type the word where the caret was, then replay the click.
func mouseProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) == 0 && (wParam == WM_LBUTTONDOWN || wParam == WM_RBUTTONDOWN || wParam == WM_MBUTTONDOWN) {
		m := (*MSLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		if m.Flags&LLMHF_INJECTED == 0 && !overWindow(popHwnd, m.Pt) && !overWindow(badgeHwnd, m.Pt) {
			prevWord = ""
			if len(buf) > 0 {
				commit("", false, nil)
				prevWord = ""
				var flag uint32 = MOUSEEVENTF_LEFTDOWN
				if wParam == WM_RBUTTONDOWN {
					flag = MOUSEEVENTF_RIGHTDOWN
				} else if wParam == WM_MBUTTONDOWN {
					flag = MOUSEEVENTF_MIDDLEDOWN
				}
				in := msInput{Type: INPUT_MOUSE, Mi: mouseInputS{DwFlags: flag, DwExtraInfo: injectedTag}}
				pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
				return 1
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

func overWindow(h uintptr, pt POINT) bool {
	if h == 0 {
		return false
	}
	if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
		return false
	}
	var r RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return pt.X >= r.Left && pt.X < r.Right && pt.Y >= r.Top && pt.Y < r.Bottom
}

// ----------------------------------------------------------------- windows

func registerClass(name string, proc interface{}, style uint32) *uint16 {
	cls := u16(name)
	arrow, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW
	wc := WNDCLASSEXW{Style: style, LpfnWndProc: syscall.NewCallback(proc), HInstance: hInstance, LpszClassName: cls, HCursor: arrow, HIcon: iconOn, HIconSm: iconOn}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	return cls
}

func createWindows() {
	cls := registerClass("MarathiTypingMain", mainProc, 0)
	mainHwnd, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(u16(appName))), 0, 0, 0, 0, 0, 0, 0, hInstance, 0)

	pcls := registerClass("MarathiTypingPopup", popupProc, CS_DROPSHADOW)
	popHwnd, _, _ = pCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, uintptr(unsafe.Pointer(pcls)), 0,
		WS_POPUP|WS_BORDER, 0, 0, 10, 10, 0, 0, hInstance, 0)

	bcls := registerClass("MarathiTypingBadge", badgeProc, 0)
	badgeHwnd, _, _ = pCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE|WS_EX_LAYERED, uintptr(unsafe.Pointer(bcls)),
		uintptr(unsafe.Pointer(u16("Marathi Typing"))), WS_POPUP, 0, 0, 10, 10, 0, 0, hInstance, 0)
	pSetLayeredWindowAttributes.Call(badgeHwnd, 0, 235, LWA_ALPHA)
}

func mainProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	m := uint32(msg)
	if m == msgTaskbarCreated && m != 0 {
		trayAdded = false
		addTray() // Explorer restarted: put the icon back
		return 0
	}
	switch m {
	case WM_HOTKEY:
		if len(buf) > 0 {
			commit("", false, nil)
		}
		toggle()
		return 0
	case WM_APP + 2:
		messageBox(welcomeText, appName+" "+version, MB_OK|MB_ICONINFORMATION)
		return 0
	case WM_TIMER:
		switch wParam {
		case idSaveTimer:
			pKillTimer.Call(hwnd, idSaveTimer)
			saveLearned()
		case idFlash:
			pKillTimer.Call(hwnd, idFlash)
			statusText = ""
			if len(buf) == 0 {
				pShowWindow.Call(popHwnd, SW_HIDE)
			}
		case idDictTimer:
			checkDict()
		case idTrayRetry:
			if trayAdded || trayTries > 15 {
				pKillTimer.Call(hwnd, idTrayRetry)
			} else {
				addTray()
			}
		}
		return 0
	case WM_TRAY:
		switch lParam & 0xFFFF {
		case WM_LBUTTONUP:
			toggle()
		case WM_RBUTTONUP:
			showMenu()
		}
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func toggle() {
	cfg.Enabled = !cfg.Enabled
	if !cfg.Enabled {
		resetBuf()
	}
	prevWord = ""
	saveSettings()
	updateTray()
	pInvalidateRect.Call(badgeHwnd, 0, 1)
	if cfg.Enabled {
		flash("मराठी")
	} else {
		flash("English")
	}
}

// flash shows a short status label near the caret.
func flash(s string) {
	statusText = s
	showPopup()
	pSetTimer.Call(mainHwnd, idFlash, 900, 0)
}

func showMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	add := func(id int, text string, on bool) {
		var fl uintptr = MF_STRING
		if on {
			fl |= MF_CHECKED
		}
		if id == 0 {
			fl = MF_SEPARATOR
		}
		pAppendMenuW.Call(m, fl, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
	}
	add(cmdToggle, "मराठी typing\tAlt+M", cfg.Enabled)
	add(0, "", false)
	add(cmdEnglish, "Keep English words in English", cfg.KeepEnglish)
	add(cmdDigits, "Marathi digits (२०२६)", cfg.Digits)
	add(cmdBadge, "Show the म / EN badge", cfg.Badge)
	add(cmdStartup, "Start with Windows", startupEnabled())
	add(0, "", false)
	add(cmdDict, "Edit my words…", false)
	add(cmdFolder, "Open settings folder", false)
	add(cmdForget, "Forget what it learned", false)
	add(cmdHelp, "How to type…", false)
	add(0, "", false)
	add(cmdExit, "Exit", false)
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(mainHwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, mainHwnd, 0)
	pDestroyMenu.Call(m)
	switch cmd {
	case cmdToggle:
		toggle()
	case cmdEnglish:
		cfg.KeepEnglish = !cfg.KeepEnglish
		eng.KeepEnglish = cfg.KeepEnglish
		saveSettings()
	case cmdDigits:
		cfg.Digits = !cfg.Digits
		saveSettings()
	case cmdBadge:
		cfg.Badge = !cfg.Badge
		saveSettings()
		if cfg.Badge {
			showBadge()
		} else {
			pShowWindow.Call(badgeHwnd, SW_HIDE)
		}
	case cmdStartup:
		setStartup(!startupEnabled())
	case cmdDict:
		loadDict() // creates the file if needed
		shellOpen("notepad.exe", `"`+dictPath()+`"`)
	case cmdFolder:
		shellOpen(dataDir, "")
	case cmdForget:
		eng.Learned = map[string]map[string]int{}
		eng.Bigram = map[string]map[string]int{}
		saveLearned()
	case cmdHelp:
		messageBox("Type Marathi the way you'd type it in a chat:\n\n"+
			"    tumhi kase aahat  →  तुम्ही कसे आहात\n\n"+
			"Space  –  type the highlighted word\n"+
			"1 – 7  –  pick another suggestion (↑ ↓ also work)\n"+
			"          words marked … are longer words that start the same way\n"+
			"Esc  –  keep the English letters\n"+
			"Alt+M  –  switch Marathi ⇄ English (or click the म badge)\n\n"+
			"Capitals for ट ड ण ळ ष:  T  D  N  L  Sh   (paaNi → पाणी)\n"+
			"It remembers the words you pick and the word you usually type next.\n"+
			"My words: right-click the badge → Edit my words… (shortcut = text).\n\n"+
			"Works in Word, Excel, PowerPoint, Outlook, Notepad, browsers and most apps.\n"+
			"Apps running as Administrator need this app to be run as Administrator too.", appName+" "+version, MB_OK|MB_ICONINFORMATION)
	case cmdExit:
		pPostQuitMessage.Call(0)
	}
}

func shellOpen(file, params string) {
	var p uintptr
	if params != "" {
		p = uintptr(unsafe.Pointer(u16(params)))
	}
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(file))), p, 0, 1)
}

// ------------------------------------------------------------------- popup

func popupProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case WM_MOUSEACTIVATE:
		return MA_NOACTIVATE
	case WM_LBUTTONDOWN:
		y := int32(int16(lParam >> 16 & 0xFFFF))
		top := sc(8) + lineH(fontLatin) + sc(6)
		row := (y - top) / rowH()
		if y >= top && int(row) < len(cands) && len(buf) > 0 {
			sel, picked = int(row), true
			commit("", false, nil)
		}
		return 0
	case WM_PAINT:
		paintPopup(hwnd)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func sc(v int32) int32 { return int32(float64(v) * dpiScale) }

func createFonts() {
	mk := func(h int32, weight int, face string) uintptr {
		f, _, _ := pCreateFontW.Call(uintptr(uint32(-sc(h))), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16(face))))
		return f
	}
	fontLatin = mk(15, 400, "Consolas")
	fontDeva = mk(22, 400, "Nirmala UI")
	fontSmall = mk(12, 400, "Segoe UI")
	fontBadge = mk(20, 700, "Nirmala UI")
}

func lineH(font uintptr) int32 {
	dc, _, _ := pGetDC.Call(popHwnd)
	old, _, _ := pSelectObject.Call(dc, font)
	var s SIZE
	pGetTextExtentPoint32W.Call(dc, uintptr(unsafe.Pointer(u16("Ag"))), 2, uintptr(unsafe.Pointer(&s)))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(popHwnd, dc)
	return s.CY
}

func rowH() int32 { return lineH(fontDeva) + sc(6) }

func textW(font uintptr, s string) int32 {
	if s == "" {
		return 0
	}
	dc, _, _ := pGetDC.Call(popHwnd)
	old, _, _ := pSelectObject.Call(dc, font)
	u, _ := syscall.UTF16FromString(s)
	var sz SIZE
	pGetTextExtentPoint32W.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(popHwnd, dc)
	return sz.CX
}

const hintText = "Space ✓   1–7 choose   Esc English"

func popupSize() (int32, int32) {
	if len(buf) == 0 {
		return textW(fontDeva, statusText) + sc(28), lineH(fontDeva) + sc(14)
	}
	w := textW(fontLatin, string(buf)) + sc(24)
	for _, c := range cands {
		if cw := textW(fontDeva, c) + sc(64); cw > w {
			w = cw
		}
	}
	if hw := textW(fontSmall, hintText) + sc(24); hw > w {
		w = hw
	}
	if w < sc(170) {
		w = sc(170)
	}
	h := sc(8) + lineH(fontLatin) + sc(6) + rowH()*int32(len(cands)) + sc(4) + lineH(fontSmall) + sc(8)
	return w, h
}

func caretPos() (int32, int32, bool) {
	fg, _, _ := pGetForegroundWindow.Call()
	tid, _, _ := pGetWindowThreadProcId.Call(fg, 0)
	var gi GUITHREADINFO
	gi.CbSize = uint32(unsafe.Sizeof(gi))
	if r, _, _ := pGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gi))); r != 0 && gi.HwndCaret != 0 &&
		(gi.RcCaret.Bottom > gi.RcCaret.Top || gi.RcCaret.Right > gi.RcCaret.Left) {
		pt := POINT{gi.RcCaret.Left, gi.RcCaret.Bottom}
		pClientToScreen.Call(gi.HwndCaret, uintptr(unsafe.Pointer(&pt)))
		return pt.X, pt.Y, true
	}
	// no system caret (many modern apps): bottom-centre of the active window
	var r RECT
	pGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&r)))
	return (r.Left + r.Right) / 2, r.Bottom - sc(220), false
}

func workArea(x, y int32) RECT {
	mon, _, _ := pMonitorFromPoint.Call(uintptr(uint32(x))|uintptr(uint32(y))<<32, 2)
	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	return mi.RcWork
}

func showPopup() {
	if len(buf) == 0 && statusText == "" {
		pShowWindow.Call(popHwnd, SW_HIDE)
		return
	}
	w, h := popupSize()
	x, y, _ := caretPos()
	y += sc(6)
	// keep on the monitor's work area
	if wa := workArea(x, y); wa.Right > wa.Left {
		if x+w > wa.Right {
			x = wa.Right - w
		}
		if x < wa.Left {
			x = wa.Left
		}
		if y+h > wa.Bottom {
			y = y - h - sc(36)
		}
		if y < wa.Top {
			y = wa.Top
		}
	}
	pSetWindowPos.Call(popHwnd, HWND_TOPMOST, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOACTIVATE|SWP_SHOWWINDOW)
	pInvalidateRect.Call(popHwnd, 0, 1)
}

func drawText(dc uintptr, font uintptr, color uintptr, s string, r RECT, flags uintptr) {
	if s == "" {
		return
	}
	pSelectObject.Call(dc, font)
	pSetTextColor.Call(dc, color)
	u, _ := syscall.UTF16FromString(s)
	pDrawTextW.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), flags)
}

func fill(dc uintptr, r RECT, color uintptr) {
	br, _, _ := pCreateSolidBrush.Call(color)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), br)
	pDeleteObject.Call(br)
}

func paintPopup(hwnd uintptr) {
	var ps PAINTSTRUCT
	dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var wr RECT
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	w := wr.Right - wr.Left
	pSetBkMode.Call(dc, TRANSPARENT)
	if len(buf) == 0 {
		fill(dc, RECT{0, 0, w, 10000}, rgb(194, 65, 12))
		drawText(dc, fontDeva, rgb(255, 255, 255), statusText, RECT{0, 0, w, wr.Bottom - wr.Top}, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)
		return
	}

	lh := lineH(fontLatin)
	head := RECT{0, 0, w, sc(8) + lh + sc(4)}
	fill(dc, RECT{0, 0, w, 10000}, rgb(255, 255, 255))
	fill(dc, head, rgb(255, 247, 237))
	drawText(dc, fontLatin, rgb(154, 52, 18), string(buf), RECT{sc(10), sc(6), w - sc(8), sc(6) + lh}, DT_LEFT|DT_SINGLELINE|DT_NOPREFIX)

	y := sc(8) + lh + sc(6)
	rh := rowH()
	for i, c := range cands {
		r := RECT{sc(4), y, w - sc(4), y + rh}
		isComp := i >= len(cands)-nComp
		if isComp && i == len(cands)-nComp {
			fill(dc, RECT{sc(10), y, w - sc(10), y + 1}, rgb(231, 229, 228)) // separator
		}
		if i == sel {
			fill(dc, r, rgb(255, 237, 213))
		}
		numColor := rgb(168, 162, 158)
		if i == sel {
			numColor = rgb(194, 65, 12)
		}
		textColor := rgb(28, 25, 23)
		if isComp {
			textColor = rgb(87, 83, 78)
			drawText(dc, fontSmall, rgb(168, 162, 158), "…", RECT{w - sc(24), y, w - sc(6), y + rh}, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		}
		drawText(dc, fontSmall, numColor, string(rune('1'+i)), RECT{sc(10), y, sc(28), y + rh}, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(dc, fontDeva, textColor, c, RECT{sc(30), y, w - sc(26), y + rh}, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)
		y += rh
	}
	y += sc(4)
	drawText(dc, fontSmall, rgb(120, 113, 108), hintText, RECT{sc(10), y, w - sc(6), y + lineH(fontSmall) + sc(4)}, DT_LEFT|DT_SINGLELINE)
}

// ------------------------------------------------------------------- badge
// A small always-on-top म / EN badge: drag to move, click to switch, right-click for the menu.

var (
	dragging   bool
	dragMoved  bool
	dragStart  POINT
	dragOrigin RECT
)

func badgeSize() int32 { return sc(40) }

func showBadge() {
	s := badgeSize()
	x, y := cfg.BadgeX, cfg.BadgeY
	var cur POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
	wa := workArea(x, y)
	if x == 0 && y == 0 || x < wa.Left || y < wa.Top || x+s > wa.Right || y+s > wa.Bottom {
		wa = workArea(cur.X, cur.Y)
		pri := workArea(1, 1) // primary monitor
		if pri.Right > pri.Left {
			wa = pri
		}
		x, y = wa.Right-s-sc(24), wa.Bottom-s-sc(24)
	}
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(s+1), uintptr(s+1), uintptr(sc(12)), uintptr(sc(12)))
	pSetWindowRgn.Call(badgeHwnd, rgn, 1)
	pSetWindowPos.Call(badgeHwnd, HWND_TOPMOST, uintptr(x), uintptr(y), uintptr(s), uintptr(s), SWP_NOACTIVATE|SWP_SHOWWINDOW)
	pInvalidateRect.Call(badgeHwnd, 0, 1)
}

func badgeProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case WM_MOUSEACTIVATE:
		return MA_NOACTIVATE
	case WM_LBUTTONDOWN:
		dragging, dragMoved = true, false
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&dragStart)))
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&dragOrigin)))
		pSetCapture.Call(hwnd)
		return 0
	case WM_MOUSEMOVE:
		if dragging {
			var p POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			dx, dy := p.X-dragStart.X, p.Y-dragStart.Y
			if dx*dx+dy*dy > 16 {
				dragMoved = true
			}
			if dragMoved {
				pSetWindowPos.Call(hwnd, HWND_TOPMOST, uintptr(dragOrigin.Left+dx), uintptr(dragOrigin.Top+dy), 0, 0, SWP_NOACTIVATE|SWP_NOSIZE)
			}
		}
		return 0
	case WM_LBUTTONUP:
		if dragging {
			dragging = false
			pReleaseCapture.Call()
			if dragMoved {
				var r RECT
				pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
				cfg.BadgeX, cfg.BadgeY = r.Left, r.Top
				saveSettings()
			} else {
				if len(buf) > 0 {
					commit("", false, nil)
				}
				toggle()
			}
		}
		return 0
	case WM_RBUTTONUP:
		showMenu()
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		s := badgeSize()
		bg, label, font := rgb(194, 65, 12), "म", fontBadge
		if !cfg.Enabled {
			bg, label, font = rgb(87, 83, 78), "EN", fontSmall
		}
		fill(dc, RECT{0, 0, s, s}, bg)
		pSetBkMode.Call(dc, TRANSPARENT)
		drawText(dc, font, rgb(255, 255, 255), label, RECT{0, 0, s, s - sc(2)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// -------------------------------------------------------------------- tray

// loadIcon writes the embedded .ico next to the settings and loads it at the
// small-icon size; falls back to drawing one if that fails.
func loadIcon(ico []byte, name string, bg uintptr) uintptr {
	p := filepath.Join(dataDir, name)
	if old, err := os.ReadFile(p); err != nil || !bytes.Equal(old, ico) {
		os.WriteFile(p, ico, 0o644)
	}
	cx, _, _ := pGetSystemMetrics.Call(SM_CXSMICON)
	cy, _, _ := pGetSystemMetrics.Call(SM_CYSMICON)
	h, _, _ := pLoadImageW.Call(0, uintptr(unsafe.Pointer(u16(p))), IMAGE_ICON, cx, cy, LR_LOADFROMFILE)
	if h != 0 {
		return h
	}
	return makeIcon(bg)
}

func makeIcon(bg uintptr) uintptr {
	const n = 32
	screen, _, _ := pGetDC.Call(0)
	dc, _, _ := pCreateCompatibleDC.Call(screen)
	bmp, _, _ := pCreateCompatibleBitmap.Call(screen, n, n)
	var maskBits [n * n / 8]byte // all zero = fully opaque
	mask, _, _ := pCreateBitmap.Call(n, n, 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	old, _, _ := pSelectObject.Call(dc, bmp)
	fill(dc, RECT{0, 0, n, n}, bg)
	fh := int32(-30)
	f, _, _ := pCreateFontW.Call(uintptr(uint32(fh)), 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16("Nirmala UI"))))
	pSetBkMode.Call(dc, TRANSPARENT)
	drawText(dc, f, rgb(255, 255, 255), "म", RECT{0, -2, n, n}, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	pSelectObject.Call(dc, old)
	pDeleteObject.Call(f)
	ii := ICONINFO{FIcon: 1, HbmMask: mask, HbmColor: bmp}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(bmp)
	pDeleteObject.Call(mask)
	pDeleteDC.Call(dc)
	pReleaseDC.Call(0, screen)
	return icon
}

func trayData() NOTIFYICONDATAW {
	var nd NOTIFYICONDATAW
	nd.CbSize = uint32(unsafe.Sizeof(nd))
	nd.HWnd = mainHwnd
	nd.UID = 1
	return nd
}

func addTray() {
	nd := trayData()
	nd.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nd.UCallbackMessage = WM_TRAY
	nd.HIcon = iconOff
	if cfg.Enabled {
		nd.HIcon = iconOn
	}
	copyU16(nd.SzTip[:], tipText())
	r, _, _ := pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nd)))
	if r == 0 {
		// maybe it already exists (e.g. after a crash) or Explorer isn't ready yet
		if m, _, _ := pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nd))); m != 0 {
			trayAdded = true
			return
		}
		trayTries++
		pSetTimer.Call(mainHwnd, idTrayRetry, 2000, 0)
		return
	}
	trayAdded = true
}

func tipText() string {
	if cfg.Enabled {
		return "मराठी typing ON (Alt+M to switch)"
	}
	return "Marathi typing OFF (Alt+M to switch)"
}

func updateTray() {
	nd := trayData()
	nd.UFlags = NIF_ICON | NIF_TIP
	nd.HIcon = iconOff
	if cfg.Enabled {
		nd.HIcon = iconOn
	}
	copyU16(nd.SzTip[:], tipText())
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nd)))
}

func balloon(title, text string) {
	nd := trayData()
	nd.UFlags = NIF_INFO
	nd.DwInfoFlags = NIIF_INFO
	copyU16(nd.SzInfoTitle[:], title)
	copyU16(nd.SzInfo[:], text)
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nd)))
}

func removeTray() {
	nd := trayData()
	pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nd)))
}

// ----------------------------------------------------------------- startup

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func startupEnabled() bool {
	var h uintptr
	if r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), 0, KEY_READ, uintptr(unsafe.Pointer(&h))); r != 0 {
		return false
	}
	defer pRegCloseKey.Call(h)
	r, _, _ := pRegQueryValueExW.Call(h, uintptr(unsafe.Pointer(u16(appName))), 0, 0, 0, 0)
	return r == 0
}

func setStartup(on bool) {
	var h uintptr
	if r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), 0, KEY_WRITE, uintptr(unsafe.Pointer(&h))); r != 0 {
		return
	}
	defer pRegCloseKey.Call(h)
	if on {
		exe, _ := os.Executable()
		v, _ := syscall.UTF16FromString(`"` + exe + `"`)
		pRegSetValueExW.Call(h, uintptr(unsafe.Pointer(u16(appName))), 0, REG_SZ, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	} else {
		pRegDeleteValueW.Call(h, uintptr(unsafe.Pointer(u16(appName))))
	}
}
