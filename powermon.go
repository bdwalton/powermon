package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/godbus/dbus/v5"
	"os/exec"
)

var (
	// CLI Flags
	actionCmd = flag.String("action", "", "Run this command when 'on battery' state changes")
	logfile   = flag.String("logfile", "", "If set, log to this path instead of the default (os.Stderr) target")
	verbose   = flag.Bool("verbose", false, "If true, output logging status updates. Be quiet when false.")
)

func maybeLog(fmt string, args ...interface{}) {
	if *verbose {
		reallyLog(fmt, args...)
	}
}

func reallyLog(fmt string, args ...interface{}) {
	log.Printf(fmt, args...)
}

const (
	// power states
	UNKNOWN = iota
	ON_BATTERY
	AC_POWER
)

type powerState uint8

var states = map[powerState]string{
	UNKNOWN:    "UNKNOWN",
	ON_BATTERY: "ON_BATTERY",
	AC_POWER:   "AC_POWER",
}

func (ps powerState) String() string {
	return states[ps]
}

// powermon represents the object that will monitor system power state
// and trigger actions on change
type powermon struct {
	// An executable command that will be run, passed an argument
	// of battery or ac to allow the command to act accordingly
	action   string
	dbusConn *dbus.Conn
	state    powerState
	quitCh   chan struct{}
}

const (
	upower     = "org.freedesktop.UPower"
	upowerPath = "/org/freedesktop/UPower"
	onBattery  = "OnBattery"
)

func newPowermon(action string) (*powermon, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus connect failed: %v", err)
	}

	obj := conn.Object(upower, upowerPath)
	var state powerState = UNKNOWN
	if ps, err := obj.GetProperty(upower + "." + onBattery); err != nil {
		reallyLog("failed to get battery state: %v", err)
	} else {
		v := ps.Value().(bool)
		switch v {
		case true:
			state = ON_BATTERY
		default:
			state = AC_POWER
		}
	}

	p := &powermon{
		dbusConn: conn,
		state:    state,
		action:   action,
		quitCh:   make(chan struct{}),
	}

	p.stateChange()

	if err := p.dbusConn.AddMatchSignal(dbus.WithMatchObjectPath(upowerPath), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchSender(upower)); err != nil {
		return nil, fmt.Errorf("couldn't setup signal listener: %v", err)
	}

	return p, nil
}

func (p *powermon) logState() {
	maybeLog("power state: %s", p.state)
}

func (p *powermon) stateChange() {
	p.logState()

	s := p.state.String()
	maybeLog("running command: %s %s", p.action, s)
	if out, err := exec.Command(p.action, s).CombinedOutput(); err != nil {
		maybeLog("error running '%s %s': %v", p.action, s, err)
		maybeLog("error output: %s", out)
	}
}

func (p *powermon) run() {
	defer close(p.quitCh)

	c := make(chan *dbus.Signal, 10)
	p.dbusConn.Signal(c)

	maybeLog("running...")
	for {
		select {
		case sig := <-c:
			val := sig.Body[1].(map[string]dbus.Variant)
			switch val[onBattery].String() {
			case "true":
				p.state = ON_BATTERY
			case "false":
				p.state = AC_POWER
			default:
				p.state = UNKNOWN
			}
			p.stateChange()
		case <-p.quitCh:
			maybeLog("shutting down main loop")
			return
		}
	}
}

func (p *powermon) shutdown() {
	p.quitCh <- struct{}{}
	<-p.quitCh
	p.dbusConn.Close()
}

func main() {
	flag.Parse()

	if *logfile != "" {
		lf, err := os.OpenFile(*logfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			log.Fatalf("Couldn't open logfile %q: %v\n", *logfile, err)
		}
		log.SetOutput(lf)
	}

	if *actionCmd == "" {
		maybeLog("No action to run on state change. Pass --action='/some/command'.")
		os.Exit(1)
	}

	prog, err := os.Executable()
	if err != nil {
		maybeLog("Error determining program executable: %v\n", err)
		os.Exit(1)
	}

	log.SetPrefix(filepath.Base(prog) + ": ")

	pm, err := newPowermon(*actionCmd)
	if err != nil {
		maybeLog("Setup failure: %v\n", err)
		os.Exit(1)
	}

	go pm.run()

	sigQuit := make(chan os.Signal, 1)
	signal.Notify(sigQuit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case s := <-sigQuit:
			maybeLog("received signal %q. shutting down...", s)
			pm.shutdown()
			maybeLog("goodbye")
			os.Exit(0)
		}
	}
}
