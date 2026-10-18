//go:build windows

package proc

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procAllocConsole      = kernel32.NewProc("AllocConsole")
	procGetConsoleWindow  = kernel32.NewProc("GetConsoleWindow")
	user32                = windows.NewLazySystemDLL("user32.dll")
	procShowWindow        = user32.NewProc("ShowWindow")
	advapi32              = windows.NewLazySystemDLL("advapi32.dll")
	procLogonUserW        = advapi32.NewProc("LogonUserW")
	userenv               = windows.NewLazySystemDLL("userenv.dll")
	procLoadUserProfileW  = userenv.NewProc("LoadUserProfileW")
	procUnloadUserProfile = userenv.NewProc("UnloadUserProfile")
)

// Init gives the platform a (hidden) console when it has none, e.g. when running as a
// service. Children share it, which is what lets us send them CTRL_BREAK for a graceful stop.
func Init() error {
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		return nil
	}
	if r, _, err := procAllocConsole.Call(); r == 0 {
		return fmt.Errorf("AllocConsole: %w", err)
	}
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		procShowWindow.Call(hwnd, 0)
	}
	return nil
}

func scriptCommand(script, dir string) (*exec.Cmd, error) {
	file, ok, err := scriptFile(script, dir)
	if err != nil {
		return nil, err
	}
	if ok {
		switch strings.ToLower(filepath.Ext(file)) {
		case ".ps1":
			return powershell("-File", file), nil
		case ".cmd", ".bat":
			return exec.Command("cmd.exe", "/d", "/c", file), nil
		default:
			return nil, fmt.Errorf("%s: .sh scripts don't run on Windows", script)
		}
	}
	// With -EncodedCommand and redirected output, Windows PowerShell writes errors and
	// progress to stderr as CLIXML. So progress is off, and errors are caught and
	// written as plain text straight to the console stream.
	full := "$ProgressPreference = 'SilentlyContinue'\n$ErrorActionPreference = 'Stop'\n$PSNativeCommandUseErrorActionPreference = $true\n" +
		"try {\n" + script + "\n} catch {\n[Console]::Error.WriteLine(($_ | Out-String).Trim())\nexit 1\n}\nexit $LASTEXITCODE\n"
	return powershell("-EncodedCommand", encodePowershell(full)), nil
}

func powershell(args ...string) *exec.Cmd {
	return exec.Command("powershell.exe", append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}, args...)...)
}

// encodePowershell produces the base64 UTF-16LE form -EncodedCommand expects,
// which sidesteps all command-line quoting issues for multi-line scripts.
func encodePowershell(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

type tree struct {
	mu     sync.Mutex
	job    windows.Handle
	pid    uint32
	closed bool
}

// start launches the process suspended, puts it in a job object and only then resumes it,
// so no grandchild can escape the job.
func (t *tree) start(cmd *exec.Cmd, u *User) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED}
	if u != nil {
		cmd.SysProcAttr.Token = syscall.Token(u.token)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	if err := cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return err
	}
	t.pid = uint32(cmd.Process.Pid)
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, t.pid)
	if err == nil {
		err = windows.AssignProcessToJobObject(job, h)
		windows.CloseHandle(h)
	}
	if err == nil {
		err = resumeProcess(t.pid)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		windows.CloseHandle(job)
		return err
	}
	t.job = job
	return nil
}

func resumeProcess(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	te := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != pid {
			continue
		}
		th, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(th)
		windows.CloseHandle(th)
		if err != nil {
			return err
		}
		resumed = true
	}
	if !resumed {
		return fmt.Errorf("resume process %d: no threads found", pid)
	}
	return nil
}

func (t *tree) interrupt() error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, t.pid)
}

func (t *tree) kill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		windows.TerminateJobObject(t.job, 1)
	}
}

func (t *tree) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		windows.TerminateJobObject(t.job, 1)
		windows.CloseHandle(t.job)
		t.closed = true
	}
}

type User struct {
	Name    string
	token   windows.Token
	profile windows.Handle
	sid     *windows.SID
}

type profileInfo struct {
	Size        uint32
	Flags       uint32
	UserName    *uint16
	ProfilePath *uint16
	DefaultPath *uint16
	ServerName  *uint16
	PolicyPath  *uint16
	Profile     windows.Handle
}

const (
	logonInteractive = 2
	logonProvider    = 0
	modifyAccess     = 0x1301bf
)

// LookupUser logs the account on and loads its profile. name may be "user",
// "DOMAIN\user" or "user@domain". The caller must Close the user.
func LookupUser(name, password string) (*User, error) {
	if password == "" {
		return nil, errors.New("a password is required to run as another user on Windows")
	}
	domain, account := ".", name
	if i := strings.IndexByte(name, '\\'); i >= 0 {
		domain, account = name[:i], name[i+1:]
	} else if strings.Contains(name, "@") {
		domain = ""
	}
	accountPtr, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return nil, err
	}
	passwordPtr, err := windows.UTF16PtrFromString(password)
	if err != nil {
		return nil, err
	}
	var domainPtr *uint16
	if domain != "" {
		if domainPtr, err = windows.UTF16PtrFromString(domain); err != nil {
			return nil, err
		}
	}

	var token windows.Token
	r, _, err := procLogonUserW.Call(uintptr(unsafe.Pointer(accountPtr)), uintptr(unsafe.Pointer(domainPtr)),
		uintptr(unsafe.Pointer(passwordPtr)), logonInteractive, logonProvider, uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return nil, fmt.Errorf("logon %s: %w", name, err)
	}

	pi := profileInfo{UserName: accountPtr}
	pi.Size = uint32(unsafe.Sizeof(pi))
	if r, _, err := procLoadUserProfileW.Call(uintptr(token), uintptr(unsafe.Pointer(&pi))); r == 0 {
		token.Close()
		return nil, fmt.Errorf("load profile for %s: %w", name, err)
	}

	tu, err := token.GetTokenUser()
	if err == nil {
		var sid *windows.SID
		if sid, err = tu.User.Sid.Copy(); err == nil {
			return &User{Name: name, token: token, profile: pi.Profile, sid: sid}, nil
		}
	}
	procUnloadUserProfile.Call(uintptr(token), uintptr(pi.Profile))
	token.Close()
	return nil, err
}

func (u *User) Close() {
	procUnloadUserProfile.Call(uintptr(u.token), uintptr(u.profile))
	u.token.Close()
}

func (u *User) environ() []string {
	env, err := u.token.Environ(false)
	if err != nil {
		return os.Environ()
	}
	return env
}

// Grant gives the user Modify rights on each path, inherited by everything inside it.
func (u *User) Grant(paths ...string) error {
	for _, path := range paths {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		current, _, err := sd.DACL()
		if err != nil {
			return err
		}
		acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
			AccessPermissions: modifyAccess,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(u.sid),
			},
		}}, current)
		if err != nil {
			return err
		}
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			return fmt.Errorf("grant %s on %s: %w", u.Name, path, err)
		}
	}
	return nil
}

// CanRunAs is checked when the app starts on Windows, where LogonUser needs the password.
func CanRunAs(string) error { return nil }
