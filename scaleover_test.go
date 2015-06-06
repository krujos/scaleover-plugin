package main

import (
	"time"

	"github.com/cloudfoundry/cli/plugin/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scaleover", func() {
	var scaleoverCmdPlugin *ScaleoverCmd
	var fakeCliConnection *fakes.FakeCliConnection

	Describe("getAppStatus", func() {

		BeforeEach(func() {
			fakeCliConnection = &fakes.FakeCliConnection{}
			scaleoverCmdPlugin = &ScaleoverCmd{}
		})

		It("should Fail Without App 1", func() {
			fakeCliConnection.CliCommandWithoutTerminalOutputReturns([]string{"FAILED", "App app1 not found"}, nil)
			var err error
			_, err = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")
			Expect(err.Error()).To(Equal("App app1 not found"))
		})

		It("should Fail Without App 2", func() {
			fakeCliConnection.CliCommandWithoutTerminalOutputReturns([]string{"FAILED", "App app2 not found"}, nil)
			var err error
			_, err = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app2")
			Expect(err.Error()).To(Equal("App app2 not found"))
		})

		It("should not start a stopped target with 1 instance", func() {
			cfAppOutput := []string{"requested state: stopped", "instances: 0/1"}
			fakeCliConnection.CliCommandWithoutTerminalOutputReturns(cfAppOutput, nil)

			var status AppStatus
			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")

			Expect(status.name).To(Equal("app1"))
			Expect(status.countRequested).To(Equal(0))
			Expect(status.countRunning).To(Equal(0))
			Expect(status.state).To(Equal("stopped"))
		})

		It("should start a started app with 10 instances", func() {
			cfAppOutput := []string{"requested state: started", "instances: 10/10"}
			fakeCliConnection.CliCommandWithoutTerminalOutputReturns(cfAppOutput, nil)

			var status AppStatus
			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")

			Expect(status.name).To(Equal("app1"))
			Expect(status.countRequested).To(Equal(10))
			Expect(status.countRunning).To(Equal(10))
			Expect(status.state).To(Equal("started"))
		})

		It("should keep a stop app stopped with 10 instances", func() {
			cfAppOutput := []string{"requested state: stopped", "instances: 0/10"}
			fakeCliConnection.CliCommandWithoutTerminalOutputReturns(cfAppOutput, nil)

			var status AppStatus
			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")

			Expect(status.name).To(Equal("app1"))
			Expect(status.countRequested).To(Equal(0))
			Expect(status.countRunning).To(Equal(0))
			Expect(status.state).To(Equal("stopped"))
		})

		It("should populate the routes for an app with one url", func() {

			cfAppOutput := []string{"requested state: stopped", "instances: 0/10", "urls: app.cfapps.io"}
			fakeCliConnection.CliCommandWithoutTerminalOutputReturns(cfAppOutput, nil)

			var status AppStatus
			status, _ = scaleoverCmdPlugin.getAppStatus(fakeCliConnection, "app1")
			Expect(len(status.routes)).To(Equal(1))
			Expect(status.routes[0]).To(Equal("app.cfapps.io"))
		})

		It("should populate the routes for an app with three urls", func() {
			cfAppOutput := []string{"requested state: stopped", "instances: 0/10",
				"urls: app.cfapps.io, foo-app.cfapps.io, foo-app-b.cfapps.io"}

			fakeCliConnection.CliCommandWithoutTerminalOutputReturns(cfAppOutput, nil)

			var status AppStatus
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
			fakeCliConnection = &fakes.FakeCliConnection{}

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
			fakeCliConnection = &fakes.FakeCliConnection{}

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
			scaleoverCmdPlugin.app1 = *app1
			scaleoverCmdPlugin.app2 = *app2
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
})
