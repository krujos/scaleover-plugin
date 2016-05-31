package main

import (
	"errors"
	"time"

	"github.com/cloudfoundry/cli/plugin/models"
	"github.com/cloudfoundry/cli/plugin/pluginfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scaleover", func() {
	var scaleoverCmdPlugin *ScaleoverCmd
	var fakeCliConnection *pluginfakes.FakeCliConnection
	var status *AppStatus
	domain := plugin_models.GetApp_DomainFields{Name: "cfapps.io"}

	Describe("getAppStatus", func() {

		BeforeEach(func() {
			fakeCliConnection = &pluginfakes.FakeCliConnection{}
			scaleoverCmdPlugin = &ScaleoverCmd{}
		})

		It("should Fail Without App 1", func() {
			app := plugin_models.GetAppModel{}
			fakeCliConnection.GetAppReturns(app, errors.New("App app1 not found"))
			//the command above shuld return nil through a cli fake?
			_, err := scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")
			Expect(err.Error()).To(Equal("App app1 not found"))
		})

		It("should not start a stopped target with 1 instance", func() {
			app := plugin_models.GetAppModel{State: "stopped"}
			fakeCliConnection.GetAppReturns(app, nil)

			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")

			Expect(status.name).To(Equal("app1"))
			Expect(status.countRequested).To(Equal(0))
			Expect(status.countRunning).To(Equal(0))
			Expect(status.state).To(Equal("stopped"))
		})

		It("should report the correct number of instances", func() {
			app := plugin_models.GetAppModel{
				InstanceCount:    10,
				RunningInstances: 10,
				State:            "started",
			}
			fakeCliConnection.GetAppReturns(app, nil)

			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")

			Expect(status.name).To(Equal("app1"))
			Expect(status.countRequested).To(Equal(10))
			Expect(status.countRunning).To(Equal(10))
		})

		It("should keep a stoped app stopped with 10 instances", func() {

			app := plugin_models.GetAppModel{
				InstanceCount:    10,
				RunningInstances: 0,
				State:            "stopped",
			}
			fakeCliConnection.GetAppReturns(app, nil)

			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")

			Expect(status.name).To(Equal("app1"))
			Expect(status.countRequested).To(Equal(0))
			Expect(status.countRunning).To(Equal(0))
			Expect(status.state).To(Equal("stopped"))
		})

		It("should report zero requested instances for a stopped app", func() {
			app := plugin_models.GetAppModel{
				InstanceCount:    10,
				RunningInstances: 10,
				State:            "stopped",
			}

			fakeCliConnection.GetAppReturns(app, nil)

			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")
			Ω(status.countRequested).To(Equal(0))
		})

		It("should populate the routes for an app with one url", func() {
			routes := []plugin_models.GetApp_RouteSummary{
				{
					Host:   "app",
					Domain: domain,
				},
			}

			app := plugin_models.GetAppModel{Routes: routes}
			fakeCliConnection.GetAppReturns(app, nil)
			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")
			Expect(len(status.routes)).To(Equal(1))
			Expect(status.routes[0]).To(Equal("app.cfapps.io"))
		})

		It("should populate the routes for an app with three urls", func() {

			routes := []plugin_models.GetApp_RouteSummary{
				{
					Host:   "app",
					Domain: domain,
				},
				{
					Host:   "foo-app",
					Domain: domain,
				},
				{
					Host:   "foo-app-b",
					Domain: domain,
				},
			}

			app := plugin_models.GetAppModel{Routes: routes}
			fakeCliConnection.GetAppReturns(app, nil)

			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")
			Expect(len(status.routes)).To(Equal(3))
			Expect(status.routes[0]).To(Equal("app.cfapps.io"))
			Expect(status.routes[1]).To(Equal("foo-app.cfapps.io"))
			Expect(status.routes[2]).To(Equal("foo-app-b.cfapps.io"))
		})

	})

	Describe("It should handle weird time inputs", func() {
		BeforeEach(func() {
			scaleoverCmdPlugin = &ScaleoverCmd{}
		})

		It("like a negative number", func() {
			var err error
			_, err = scaleoverCmdPlugin.parseTime("-1m")

			Expect(err.Error()).To(Equal("Duration must be a positive number in the format of 1m"))
		})

		It("like zero", func() {
			var t time.Duration
			t, _ = scaleoverCmdPlugin.parseTime("0m")
			one, _ := time.ParseDuration("0s")
			Expect(t).To(Equal(one))
		})
	})

	Describe("scale up", func() {
		var appStatus *AppStatus

		BeforeEach(func() {
			appStatus = &AppStatus{
				name:           "foo",
				countRequested: 1,
				countRunning:   1,
				state:          "stopped",
			}
			fakeCliConnection = &pluginfakes.FakeCliConnection{}

		})

		It("Starts a stopped app", func() {
			appStatus.scaleUp(fakeCliConnection)
			Expect(appStatus.state).To(Equal("started"))
		})

		It("It increments the amount requested", func() {
			running := appStatus.countRunning
			appStatus.scaleUp(fakeCliConnection)
			Expect(appStatus.countRequested).To(Equal(running + 1))
		})

		It("Leaves a started app started", func() {
			appStatus.state = "started"
			appStatus.scaleUp(fakeCliConnection)
			Expect(appStatus.state).To(Equal("started"))
		})

	})

	Describe("scale down", func() {
		var appStatus *AppStatus

		BeforeEach(func() {
			appStatus = &AppStatus{
				name:           "foo",
				countRequested: 1,
				countRunning:   1,
				state:          "started",
			}
			fakeCliConnection = &pluginfakes.FakeCliConnection{}

		})

		It("Stops a started app going to zero instances", func() {
			appStatus.scaleDown(fakeCliConnection)
			Expect(appStatus.state).To(Equal("stopped"))
		})

		It("It decrements the amount requested", func() {
			running := appStatus.countRunning
			appStatus.scaleDown(fakeCliConnection)
			Expect(appStatus.countRequested).To(Equal(running - 1))
		})

		It("Leaves a stopped app stopped", func() {
			appStatus.state = "stopped"
			appStatus.scaleDown(fakeCliConnection)
			Expect(appStatus.state).To(Equal("stopped"))
		})

		It("Scales down the app", func() {
			appStatus.countRequested = 2
			appStatus.scaleDown(fakeCliConnection)
			Expect(appStatus.countRunning).To(Equal(1))
			Expect(fakeCliConnection.CliCommandWithoutTerminalOutputCallCount()).To(Equal(1))
		})
	})

	Describe("Usage", func() {
		BeforeEach(func() {
			scaleoverCmdPlugin = &ScaleoverCmd{}
		})

		It("shows usage for too few arguments", func() {
			Expect(scaleoverCmdPlugin.usage([]string{"scaleover"})).NotTo(BeNil())
		})

		It("shows usage for too many arguments", func() {
			Expect(scaleoverCmdPlugin.usage([]string{"scaleover", "two", "three", "1m", "foo"})).NotTo(BeNil())
		})

		It("is just right", func() {
			Expect(scaleoverCmdPlugin.usage([]string{"scaleover", "two", "three", "1m"})).To(BeNil())
		})

		It("is okay with --no-route-check at the end", func() {
			Expect(scaleoverCmdPlugin.usage([]string{"scaleover", "two", "three", "1m", "--no-route-check"})).To(BeNil())
		})

		It("is gives usage with --no-route-check in an unusual position", func() {
			Expect(scaleoverCmdPlugin.usage([]string{"scaleover", "two", "--no-route-check", "three", "1m"})).ToNot(BeNil())
		})

	})

	Describe("Routes", func() {
		BeforeEach(func() {
			scaleoverCmdPlugin = &ScaleoverCmd{}
			var app1 = &AppStatus{
				routes: []string{"a.b.c", "b.c.d"},
			}
			var app2 = &AppStatus{
				routes: []string{"c.d.e", "d.e.f"},
			}
			scaleoverCmdPlugin.app1 = app1
			scaleoverCmdPlugin.app2 = app2
		})

		It("should return false if the apps don't share a route", func() {
			Expect(scaleoverCmdPlugin.appsShareARoute()).To(BeFalse())
		})

		It("should return true when they share a route", func() {
			scaleoverCmdPlugin.app2 = scaleoverCmdPlugin.app1
			Expect(scaleoverCmdPlugin.appsShareARoute()).To(BeTrue())
		})

		It("Should warn when apps don't share a route", func() {
			Expect(scaleoverCmdPlugin.errorIfNoSharedRoute().Error()).To(Equal("Apps do not share a route!"))
		})

		It("Should be just fine if apps share a route", func() {
			scaleoverCmdPlugin.app2.routes = append(scaleoverCmdPlugin.app2.routes, "a.b.c")
			Expect(scaleoverCmdPlugin.errorIfNoSharedRoute()).To(BeNil())
		})

		It("Should ignore route sanity if --no-route-check is at the end of args", func() {
			enforceRoutes := scaleoverCmdPlugin.shouldEnforceRoutes([]string{"scaleover", "two", "three", "1m", "--no-route-check"})
			Expect(enforceRoutes).To(BeFalse())
		})

		It("Should carfuly consider routes if --no-route-check is not in the args", func() {
			enforceRoutes := scaleoverCmdPlugin.shouldEnforceRoutes([]string{"scaleover", "two", "three", "1m"})
			Expect(enforceRoutes).To(BeTrue())
		})
	})

	Describe("Do Scaleover", func() {
		BeforeEach(func() {
			scaleoverCmdPlugin = &ScaleoverCmd{}
			var app1 = &AppStatus{
				countRunning:   10,
				countRequested: 10,
			}
			var app2 = &AppStatus{
				countRunning:   0,
				countRequested: 0,
			}
			scaleoverCmdPlugin.app1 = app1
			scaleoverCmdPlugin.app2 = app2
		})

		It("should scale app2 to 10", func() {
			scaleoverCmdPlugin.doScaleover(fakeCliConnection, 10, 0, 0)
			Ω(scaleoverCmdPlugin.app2.countRequested).To(Equal(10))
		})		
	})
})
