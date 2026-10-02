//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// winPTY runs a command behind a Windows pseudoconsole (ConPTY). Unlike a
// Unix PTY, the output pipe stays open after the child exits until the
// pseudoconsole is closed, so a watcher goroutine closes it on exit and
// the caller's Read loop keeps draining meanwhile - ClosePseudoConsole
// can block on older Windows until pending output is consumed.
//
// The child is created suspended and assigned to a Job Object before it
// runs, so every descendant it spawns is owned by the job. killTree and
// Close terminate the job; KILL_ON_JOB_CLOSE covers a host that dies
// without cleaning up.
type winPTY struct {
	console windows.Handle
	in      *os.File
	out     *os.File
	process windows.Handle
	job     windows.Handle

	consoleOnce sync.Once
	closeOnce   sync.Once
	exited      chan struct{}
	exitCode    int
	exitErr     error
}

func startPTY(args []string, cols, rows int) (ptyProcess, error) {
	executable, err := exec.LookPath(args[0])
	if err != nil {
		return nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{executable}, args[1:]...)))
	if err != nil {
		return nil, err
	}
	applicationName, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return nil, err
	}

	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		_ = windows.CloseHandle(inRead)
		_ = windows.CloseHandle(inWrite)
		return nil, err
	}
	p := &winPTY{
		in:     os.NewFile(uintptr(inWrite), "conpty-in"),
		out:    os.NewFile(uintptr(outRead), "conpty-out"),
		exited: make(chan struct{}),
	}

	err = windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inRead, outWrite, 0, &p.console)
	// The pseudoconsole holds its own references to its ends of the pipes.
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)
	if err != nil {
		_ = p.in.Close()
		_ = p.out.Close()
		return nil, fmt.Errorf("create pseudoconsole: %w", err)
	}

	if err := p.spawn(applicationName, commandLine); err != nil {
		p.closeConsole()
		_ = p.in.Close()
		_ = p.out.Close()
		return nil, err
	}
	go p.watch()
	return p, nil
}

func (p *winPTY) spawn(applicationName, commandLine *uint16) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("configure job object: %w", err)
	}

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	defer attributes.Delete()
	// PSEUDOCONSOLE takes the HPCON value itself as lpValue, not a pointer
	// to it.
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&p.console)), unsafe.Sizeof(p.console)); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}

	startup := windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: attributes.List(),
	}
	var info windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(applicationName, commandLine, nil, nil, false, flags, nil, nil, &startup.StartupInfo, &info); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	defer func() { _ = windows.CloseHandle(info.Thread) }()

	if err := windows.AssignProcessToJobObject(job, info.Process); err != nil {
		_ = windows.TerminateProcess(info.Process, 1)
		_ = windows.CloseHandle(info.Process)
		_ = windows.CloseHandle(job)
		return fmt.Errorf("assign process to job object: %w", err)
	}
	if _, err := windows.ResumeThread(info.Thread); err != nil {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(info.Process)
		_ = windows.CloseHandle(job)
		return fmt.Errorf("resume process: %w", err)
	}
	p.process = info.Process
	p.job = job
	return nil
}

func (p *winPTY) watch() {
	defer close(p.exited)
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		p.exitErr = err
	} else {
		var code uint32
		if err := windows.GetExitCodeProcess(p.process, &code); err != nil {
			p.exitErr = err
		}
		p.exitCode = int(code)
	}
	p.closeConsole()
}

func (p *winPTY) closeConsole() {
	p.consoleOnce.Do(func() { windows.ClosePseudoConsole(p.console) })
}

func (p *winPTY) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *winPTY) Write(b []byte) (int, error) { return p.in.Write(b) }

func (p *winPTY) resize(cols, rows int) error {
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (p *winPTY) wait() (int, error) {
	<-p.exited
	return p.exitCode, p.exitErr
}

func (p *winPTY) killTree() {
	_ = windows.TerminateJobObject(p.job, 1)
}

// Close tears the session down in the order Microsoft documents for
// ConPTY: terminate the job so nothing keeps producing output, drain the
// output pipe while the pseudoconsole closes, then release the pipes and
// handles.
func (p *winPTY) Close() error {
	var err error
	p.closeOnce.Do(func() {
		p.killTree()
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			buf := make([]byte, 4096)
			for {
				if _, readErr := p.out.Read(buf); readErr != nil {
					return
				}
			}
		}()
		p.closeConsole()
		select {
		case <-drained:
		case <-time.After(2 * time.Second):
		}
		err = errors.Join(p.out.Close(), p.in.Close())
		<-p.exited
		_ = windows.CloseHandle(p.process)
		_ = windows.CloseHandle(p.job)
	})
	return err
}

// resizeEvents polls the console size because Windows delivers no
// SIGWINCH; Go's os/signal has no resize notification there.
func resizeEvents(stdin *os.File) (<-chan struct{}, func()) {
	events := make(chan struct{}, 1)
	events <- struct{}{}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastCols, lastRows, _ := term.GetSize(int(stdin.Fd()))
		for {
			select {
			case <-ticker.C:
				cols, rows, err := term.GetSize(int(stdin.Fd()))
				if err != nil || (cols == lastCols && rows == lastRows) {
					continue
				}
				lastCols, lastRows = cols, rows
				select {
				case events <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()
	return events, func() { close(done) }
}

// Windows has no SIGUSR1/SIGUSR2; reclaim and status stay on the Ctrl-]
// prefix and the control console.
func notifySoftSignals(chan<- os.Signal) {}

func softSignalAction(os.Signal) softAction { return softNone }
