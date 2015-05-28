package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andrew-d/go-termutil"
	"github.com/cloudfoundry/cli/plugin"
)

type AppStatus struct {
	name           string
	countRunning   int
	countRequested int
	state          string
}

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
			Major: 0,
			Minor: 0,
			Build: 1,
		},
		Commands: []plugin.Command{
			{
				Name:     "scaleover",
				HelpText: "Roll http traffic from one application to another",
				UsageDetails: plugin.Usage{
					Usage: "cf scaleover APP1 APP2 TIME",
				},
			},
		},
	}
}

func main() {
	plugin.Start(new(ScaleoverCmd))
}

func (cmd *ScaleoverCmd) Run(cliConnection plugin.CliConnection, args []string) {

	rolloverTime := time.Duration(0)
	var err error
	if (len(args) > 3) {
		rolloverTime, err = time.ParseDuration(args[3])
		if (err != nil) {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// The getAppStatus calls will exit with an error if the named apps don't exist
	cmd.app1 = cmd.getAppStatus(cliConnection, args[1])
	cmd.app2 = cmd.getAppStatus(cliConnection, args[2])

	cmd.showStatus()

	count := cmd.app1.countRequested
	sleepInterval := time.Duration(rolloverTime.Nanoseconds() / int64(count))

	for count > 0 {
		count--
		cmd.app2.scaleUp(cliConnection)
		cmd.app1.scaleDown(cliConnection)
		cmd.showStatus()
		if (count > 0) {
			time.Sleep(sleepInterval)
		}
	}
	fmt.Println()
}

func (cmd *ScaleoverCmd) getAppStatus(cliConnection plugin.CliConnection, name string) AppStatus {
	var state string
	var countRunning int
	var countRequested int
	output, err := cliConnection.CliCommandWithoutTerminalOutput("app", name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, v := range output {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "requested state: ") {
			state = strings.TrimPrefix(v, "requested state: ")
		}
		if strings.HasPrefix(v, "instances: ") {
			instances := strings.TrimPrefix(v, "instances: ")
			split := strings.Split(instances, "/")
			countRunning, _ = strconv.Atoi(split[0])
			countRequested, _ = strconv.Atoi(split[1])
		}
	}
	// Compensate for some CF weirdness that leaves the requested instances non-zero
	// even though the app is stopped
	if state == "stopped" {
		countRequested = 0
	}
	return AppStatus{
		name:           name,
		countRunning:   countRunning,
		countRequested: countRequested,
		state:          state,
	}
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
