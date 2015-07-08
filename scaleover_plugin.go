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
	app1     AppStatus
	app2     AppStatus
	maxcount int
}

//GetMetadata returns metatada
func (cmd *ScaleoverCmd) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "scaleover",
		Version: plugin.VersionType{
			Major: 1,
			Minor: 0,
			Build: 2,
		},
		Commands: []plugin.Command{
			{
				Name:     "scaleover",
				HelpText: "Roll http traffic from one application to another",
				UsageDetails: plugin.Usage{
					Usage: "cf scaleover APP1 APP2 ROLLOVER_DURATION [--no-route-check]",
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

	if 5 == len(args) {
		if "--no-route-check" == args[4] {
			badArgs = false
		}
	}

	if badArgs {
		return errors.New("Usage: cf scaleover\n\tcf scaleover APP1 APP2 ROLLOVER_DURATION [--no-route-check]")
	}
	return nil
}

func (cmd *ScaleoverCmd) shouldEnforceRoutes(args []string) bool {
	return "--no-route-check" != args[len(args)-1]
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
	enforceRoutes := cmd.shouldEnforceRoutes(args)

	if err := cmd.usage(args); nil != err {
		fmt.Println(err)
		os.Exit(1)
	}

	rolloverTime, err := cmd.parseTime(args[3])
	if nil != err {
		fmt.Println(err)
		os.Exit(1)
	}

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
		fmt.Println("There are no instances of the source app to scale over")
		os.Exit(0)
	}
	sleepInterval := time.Duration(rolloverTime.Nanoseconds() / int64(count))

	for count > 0 {
		count--
		cmd.app2.scaleUp(cliConnection)
		cmd.app1.scaleDown(cliConnection)
		cmd.showStatus()
		if count > 0 {
			time.Sleep(sleepInterval)
		}
	}
	fmt.Println()
}

func (cmd *ScaleoverCmd) getAppStatus(cliConnection plugin.CliConnection, name string) (AppStatus, error) {
	status := AppStatus{
		name:           name,
		countRunning:   0,
		countRequested: 0,
		state:          "unknown",
		routes:         []string{},
	}

	output, _ := cliConnection.CliCommandWithoutTerminalOutput("app", name)

	for idx, v := range output {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "FAILED") {
			e := output[idx+1]
			return status, errors.New(e)
		}
		if strings.HasPrefix(v, "requested state: ") {
			status.state = strings.TrimPrefix(v, "requested state: ")
		}
		if strings.HasPrefix(v, "instances: ") {
			instances := strings.TrimPrefix(v, "instances: ")
			split := strings.Split(instances, "/")
			status.countRunning, _ = strconv.Atoi(split[0])
			status.countRequested, _ = strconv.Atoi(split[1])
		}
		if strings.HasPrefix(v, "urls: ") {
			urls := strings.TrimPrefix(v, "urls: ")
			status.routes = strings.Split(urls, ", ")
		}
	}
	// Compensate for some CF weirdness that leaves the requested instances non-zero
	// even though the app is stopped
	if "stopped" == status.state {
		status.countRequested = 0
	}
	return status, nil
}

func (app *AppStatus) scaleUp(cliConnection plugin.CliConnection) {
	// If not already started, start it
	if app.state != "started" {
		cliConnection.CliCommandWithoutTerminalOutput("start", app.name)
		app.state = "started"
	}
	app.countRequested++
	cliConnection.CliCommandWithoutTerminalOutput("scale", "-i", strconv.Itoa(app.countRequested), app.name)
}

func (app *AppStatus) scaleDown(cliConnection plugin.CliConnection) {
	app.countRequested--
	// If going to zero, stop the app
	if app.countRequested == 0 {
		cliConnection.CliCommandWithoutTerminalOutput("stop", app.name)
		app.state = "stopped"
	} else {
		cliConnection.CliCommandWithoutTerminalOutput("scale", "-i", strconv.Itoa(app.countRequested), app.name)
	}
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
