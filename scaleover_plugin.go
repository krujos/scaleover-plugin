// Copyright 2016 Joshua Kruck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andrew-d/go-termutil"
	"github.com/cloudfoundry/cli/plugin"
)

//AppStatus represents the sattus of a app in CF
type AppStatus struct {
	name           string
	countRunning   int
	countRequested int
	state          string
	routes         []string
}

//ScaleoverCmd is this plugin
type ScaleoverCmd struct {
	app1     *AppStatus
	app2     *AppStatus
	maxcount int
}

//GetMetadata returns metatada
func (cmd *ScaleoverCmd) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "scaleover",
		Version: plugin.VersionType{
			Major: 1,
			Minor: 1,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "scaleover",
				HelpText: "Roll traffic from one application to another",
				UsageDetails: plugin.Usage{
					Usage: "cf scaleover APP1 APP2 ROLLOVER_DURATION",
          Options: map[string]string{
						"-no-route-check":   "Since both apps are live at the same time, there is assumed use of a shared route (default true)",
						"-wait-for-start":   "Should scaleover wait for confirmation that the scaled up instace(s) are 'started' before scaling down? (default false)",
            "-post-start-sleep": "How long should scaleover wait after the new instances are considered 'started' for the app itself to initialize/bootstrap. Supports standard duration strings (eg '10s', '1m', etc). Used ONLY in conjunction with `--wait-for-start`. (default 0)",
            "-leave":            "How many 'blue' instances should remain running when using scaleover to 'green'? (default 1/stopped)",
            "-batch-size":       "How many instances should be scaled (both up/down) at a time? (default 1)",
					},
				},
			},
		},
	}
}

func main() {
	plugin.Start(new(ScaleoverCmd))
}

func (cmd *ScaleoverCmd) usage(args []string) error {
  badArgs := 4 != len(args)

	for i := 4; i < len(args); i++ {
		fmt.Println(args[i])

		switch args[i] {
			case "--no-route-check":
				badArgs = false
			case "--leave":
				if i < len(args) -1 {
					_, err := strconv.Atoi(args[i + 1])
					badArgs = err != nil
				}
      case "--batch-size":
				if i < len(args) -1 {
					_, err := strconv.Atoi(args[i + 1])
          badArgs = err != nil
				}
      case "--post-start-sleep":
				if i < len(args) -1 {
          badArgs = args[i + 1] == ""          
				}
		}
	}

	if badArgs {
		return errors.New("Usage: cf scaleover\n\tcf scaleover APP1 APP2 ROLLOVER_DURATION [--no-route-check] [--leave N]")
	}

	return nil
}

func (cmd *ScaleoverCmd) parseArgs(args []string) (bool, int, bool, time.Duration, int) {
	enforceRoutes := true
  waitForStarted := false
  batchSize :=1
	leave := 0
  var postStartSleep time.Duration
  var err error

	for i := 4; i < len(args); i++ {
		switch(args[i]) {
			case "--no-route-check":
				enforceRoutes = false
      case "--wait-for-start":
				waitForStarted = true
      case "--leave":
				if i < len(args) -1 {
          leave, err = strconv.Atoi(args[i + 1])
				}
      case "--batch-size":
				if i < len(args) -1 {
          batchSize, err = strconv.Atoi(args[i + 1])
				}
      case "--post-start-sleep":
				if i < len(args) -1 {
          postStartSleep, err = cmd.parseTime(args[i + 1])
				}
		}
	}

  if err == nil {
		return enforceRoutes, leave, waitForStarted, postStartSleep, batchSize
	}

  //defaults on error
  return enforceRoutes, 0, waitForStarted, postStartSleep, 1
}

func (cmd *ScaleoverCmd) parseTime(duration string) (time.Duration, error) {
	rolloverTime := time.Duration(0)
	var err error
	rolloverTime, err = time.ParseDuration(duration)

	if err != nil {
		return rolloverTime, err
	}
	if 0 > rolloverTime {
		return rolloverTime, errors.New("Duration must be a positive number in the format of 1m")
	}

	return rolloverTime, nil
}

//Run runs the plugin
func (cmd *ScaleoverCmd) Run(cliConnection plugin.CliConnection, args []string) {
	if args[0] == "scaleover" {
		cmd.ScaleoverCommand(cliConnection, args)
	}
}

//ScaleoverCommand creates a new instance of this plugin
func (cmd *ScaleoverCmd) ScaleoverCommand(cliConnection plugin.CliConnection, args []string) {
	if err := cmd.usage(args); nil != err {
		fmt.Println(err)
		os.Exit(1)
	}

	rolloverTime, err := cmd.parseTime(args[3])
	if nil != err {
		fmt.Println(err)
		os.Exit(1)
	}

	enforceRoutes, leave, waitForStarted, postStartSleep, batchSize := cmd.parseArgs(args)

	// The getAppStatus calls will exit with an error if the named apps don't exist
	if cmd.app1, err = cmd.getAppStatus(cliConnection, args[1]); nil != err {
		fmt.Println(err)
		os.Exit(1)
	}

	if cmd.app2, err = cmd.getAppStatus(cliConnection, args[2]); nil != err {
		fmt.Println(err)
		os.Exit(1)
	}

	if enforceRoutes {
		if err = cmd.errorIfNoSharedRoute(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	cmd.showStatus()

	count := cmd.app1.countRequested
	if count == 0 {
		fmt.Println("\nThere are no instances of the source app to scale over\n")
		os.Exit(0)
	}
	sleepInterval := time.Duration(rolloverTime.Nanoseconds() / int64(count))

	cmd.doScaleover(cliConnection, count, leave, sleepInterval, waitForStarted, postStartSleep, batchSize)
	fmt.Println()
}

func (cmd *ScaleoverCmd) doScaleover(cliConnection plugin.CliConnection,
	count int, leave int, sleepInterval time.Duration, waitForStarted bool, postStartSleep time.Duration, batchSize int) {
  origInstances := count
	count -= cmd.app2.countRunning
	leave -= cmd.app2.countRunning

  for count > 0 {
    cmd.app2.scaleUp(cliConnection, origInstances, batchSize)
    cmd.app1.scaleDown(cliConnection, leave, batchSize, cmd.app2, waitForStarted, postStartSleep)

		count-=batchSize
		cmd.showStatus()
		if count > 0 {
			time.Sleep(sleepInterval)
		}
	}
}

func (cmd *ScaleoverCmd) getAppStatus(cliConnection plugin.CliConnection, name string) (*AppStatus, error) {
	app, err := cliConnection.GetApp(name)
	if nil != err {
		return nil, err
	}

	status := &AppStatus{
		name:           name,
		countRunning:   0,
		countRequested: 0,
		state:          "unknown",
		routes:         make([]string, len(app.Routes)),
	}

	status.state = app.State
	if app.State != "stopped" {
		status.countRequested = app.InstanceCount
	}
	status.countRunning = app.RunningInstances
	for idx, route := range app.Routes {
		status.routes[idx] = route.Host + "." + route.Domain.Name
	}
	return status, nil
}

func (app *AppStatus) scaleUp(cliConnection plugin.CliConnection, origInstances int, batchSize int) {
	app.countRequested+=batchSize
  if(app.countRequested > origInstances){
    app.countRequested = origInstances;
  }
  cliConnection.CliCommandWithoutTerminalOutput("scale", "-i", strconv.Itoa(app.countRequested), app.name)

  // If not already started, start it
	if app.state == "stopped" {
		cliConnection.CliCommandWithoutTerminalOutput("start", app.name)
		app.state = "started"
	}
}

func (app *AppStatus) scaleDown(cliConnection plugin.CliConnection, leave int,
  batchSize int, app2 *AppStatus, waitForStarted bool, postStartSleep time.Duration) {

  if waitForStarted {
    for {
      refreshedApp2Status, err := cliConnection.GetApp(app2.name)
      if nil != err {
    		fmt.Println("Can't scale down. Error")
        break
    	}

      // iterate through instances
      hasStarting := false
      for _,instance := range refreshedApp2Status.Instances {
        if(instance.State != "running") {
          hasStarting = true
          time.Sleep(1 * time.Second)
          break //instance loop
        }
      }

      if !hasStarting {
        time.Sleep(postStartSleep)
        break
      } // while loop
    }
  }

  app.countRequested-=batchSize

  if(app.countRequested < leave) {
    app.countRequested = leave
  }

	// If going to zero, stop the app, and force to one instance
	if app.countRequested <= 0 {
    app.countRequested = 1
		cliConnection.CliCommandWithoutTerminalOutput("stop", app.name)
		app.state = "stopped"
	}

	cliConnection.CliCommandWithoutTerminalOutput("scale", "-i", strconv.Itoa(app.countRequested), app.name)

}

func (cmd *ScaleoverCmd) showStatus() {
	if termutil.Isatty(os.Stdout.Fd()) {
		fmt.Printf("%s (%s) %s %s %s (%s) \r",
			cmd.app1.name,
			cmd.app1.state,
			strings.Repeat("<", cmd.app1.countRequested),
			strings.Repeat(">", cmd.app2.countRequested),
			cmd.app2.name,
			cmd.app2.state,
		)
	} else {
		fmt.Printf("%s (%s) %d instances, %s (%s) %d instances\n",
			cmd.app1.name,
			cmd.app1.state,
			cmd.app1.countRequested,
			cmd.app2.name,
			cmd.app2.state,
			cmd.app2.countRequested,
		)
	}
}

func (cmd *ScaleoverCmd) appsShareARoute() bool {
	for _, r1 := range cmd.app1.routes {
		for _, r2 := range cmd.app2.routes {
			if r1 == r2 {
				return true
			}
		}
	}
	return false
}

func (cmd *ScaleoverCmd) errorIfNoSharedRoute() error {
	if cmd.appsShareARoute() {
		return nil
	}
	return errors.New("Apps do not share a route!")
}
