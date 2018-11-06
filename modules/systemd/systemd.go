// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package systemd provides modules for watching the status of a systemd unit.
package systemd // import "barista.run/modules/systemd"

import (
	"strings"
	"time"

	"barista.run/bar"
	"barista.run/base/value"
	"barista.run/base/watchers/dbus"
	"barista.run/outputs"
	"barista.run/timing"

	systemdbus "github.com/coreos/go-systemd/dbus"
)

// State represents possible states of a systemd unit.
type State string

const (
	// StateUnknown indicates an unknown unit state.
	StateUnknown = State("")
	// StateActive indicates that unit is active.
	StateActive = State("active")
	// StateReloading indicates that the unit is active and currently reloading
	// its configuration.
	StateReloading = State("reloading")
	// StateInactive indicates that it is inactive and the previous run was
	// successful or no previous run has taken place yet.
	StateInactive = State("inactive")
	// StateFailed indicates that it is inactive and the previous run was not
	// successful.
	StateFailed = State("failed")
	// StateActivating indicates that the unit has previously been inactive but
	// is currently in the process of entering an active state.
	StateActivating = State("activating")
	// StateDeactivating indicates that the unit is currently in the process of
	// deactivation.
	StateDeactivating = State("deactivating")
)

// UnitInfo includes common information present in both services and timers.
type UnitInfo struct {
	ID          string
	Description string
	State       State
	SubState    string
	Since       time.Time
	// DBus 'call' method to control the unit.
	call func(string, ...interface{}) ([]interface{}, error)
}

// Start enqeues a start job, and possibly depending jobs.
func (u UnitInfo) Start() {
	u.call("Start", "fail")
}

// Stop stops the specified unit rather than starting it.
func (u UnitInfo) Stop() {
	u.call("Stop", "fail")
}

// Restart restarts a unit. If a service is restarted that isn't running it will
// be started.
func (u UnitInfo) Restart() {
	u.call("Restart", "fail")
}

// Reload reloads a unit. Reloading is done only if the unit is already running
// and fails otherwise.
func (u UnitInfo) Reload() {
	u.call("Reload", "fail")
}

// replaced in tests.
var busType = dbus.System

func watchUnit(unitName string) *dbus.PropertiesWatcher {
	escapedName := systemdbus.PathBusEscape(unitName)
	unitPath := "/org/freedesktop/systemd1/unit/" + escapedName
	return dbus.WatchProperties(busType,
		"org.freedesktop.systemd1", unitPath, "org.freedesktop.systemd1.Unit").
		Add("ActiveState", "SubState", "Id", "Description").
		FetchOnSignal("StateChangeTimestamp")
}

// ServiceInfo represents the state of a systemd service.
type ServiceInfo struct {
	UnitInfo
	Type    string
	ExecPID uint32
	MainPID uint32
}

// ServiceModule watches a systemd service and updates on status change
type ServiceModule struct {
	name       string
	outputFunc value.Value
}

// Service creates a module that watches the status of a systemd service.
func Service(name string) *ServiceModule {
	s := &ServiceModule{name: name}
	s.Output(func(i ServiceInfo) bar.Output {
		if i.Since.IsZero() {
			return outputs.Textf("%s (%s)", i.State, i.SubState)
		}
		since := i.Since.Format("15:04")
		if timing.Now().Add(-24 * time.Hour).After(i.Since) {
			since = i.Since.Format("Jan 2")
		}
		return outputs.Textf("%s (%s) since %s", i.State, i.SubState, since)
	})
	return s
}

// Output configures a module to display the output of a user-defined function.
func (s *ServiceModule) Output(outputFunc func(ServiceInfo) bar.Output) *ServiceModule {
	s.outputFunc.Set(outputFunc)
	return s
}

const serviceIface = "org.freedesktop.systemd1.Service"

// Stream starts the module.
func (s *ServiceModule) Stream(sink bar.Sink) {
	w := watchUnit(s.name + ".service")
	defer w.Unsubscribe()

	w.FetchOnSignal(
		serviceIface+".Type",
		serviceIface+".MainPID",
		serviceIface+".ExecMainPID",
	)

	outputFunc := s.outputFunc.Get().(func(ServiceInfo) bar.Output)
	nextOutputFunc, done := s.outputFunc.Subscribe()
	defer done()

	info := getServiceInfo(w)
	for {
		sink.Output(outputFunc(info))
		select {
		case <-w.Updates:
			info = getServiceInfo(w)
		case <-nextOutputFunc:
			outputFunc = s.outputFunc.Get().(func(ServiceInfo) bar.Output)
		}
	}
}

const usecInSec = 1000 * 1000

func getUnitInfo(w *dbus.PropertiesWatcher) (UnitInfo, map[string]interface{}) {
	u := UnitInfo{call: w.Call}
	props := w.Get()
	if s, ok := props["ActiveState"].(string); ok {
		u.State = State(s)
	}
	u.ID, _ = props["Id"].(string)
	u.Description, _ = props["Description"].(string)
	u.SubState, _ = props["SubState"].(string)
	if t, _ := props["StateChangeTimestamp"].(uint64); t > 0 {
		sec := int64(t / usecInSec)
		usec := int64(t % usecInSec)
		u.Since = time.Unix(sec, usec*1000 /* nsec */)
	}
	return u, props
}

func getServiceInfo(w *dbus.PropertiesWatcher) ServiceInfo {
	i := ServiceInfo{}
	var props map[string]interface{}
	i.UnitInfo, props = getUnitInfo(w)
	i.ID = strings.TrimSuffix(i.ID, ".service")
	if mPid, ok := props[serviceIface+".MainPID"].(uint32); ok {
		i.MainPID = mPid
	}
	if ePid, ok := props[serviceIface+".ExecMainPID"].(uint32); ok {
		i.ExecPID = ePid
	}
	i.Type, _ = props[serviceIface+".Type"].(string)
	return i
}