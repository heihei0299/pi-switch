package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
)

type nativeDoctorResult struct {
	checks []application.DoctorCheck
	issues []application.ValidationIssue
}

type nativeDoctorPage struct {
	result       nativeDoctorResult
	busy, loaded bool
	message      string
}

func (p *nativeDoctorPage) run(shell *desktopShell) {
	if p.busy {
		return
	}
	p.busy = shell.runStandardTask("doctor", func() (interface{}, error) {
		checks, issues := application.DiagnoseConfig(config.ResolvePath())
		return nativeDoctorResult{checks: checks, issues: issues}, nil
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.result, p.loaded, p.message = result.(nativeDoctorResult), true, "Diagnostics completed without writing config."
	})
}

func (p *nativeDoctorPage) view(shell *desktopShell, c *ui.Context) {
	ui.Column(c).Padding(24).Gap(12).Children(func() {
		ui.Text(c, "Doctor").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		if ui.Button(c, "Run diagnostics").Disabled(p.busy).Clicked() {
			p.run(shell)
		}
		if p.busy {
			ui.Text(c, "Checking config…").Role(ui.RoleStatus)
		}
		if !p.loaded {
			ui.Text(c, "Diagnostics not run yet.")
		}
		for _, check := range p.result.checks {
			prefix := "PASS: "
			if !check.OK {
				prefix = "FAIL: "
			}
			message := check.Message
			if len(p.result.issues) > 0 && p.result.issues[0].Path == "config" {
				message = "Config cannot be read. Check file syntax and permissions; raw values are not shown."
			}
			ui.Text(c, prefix+message)
		}
		for _, issue := range p.result.issues {
			message := issue.Message
			if issue.Path == "config" {
				message = "Config unreadable; no settings or profiles have been changed."
			}
			ui.Text(c, issue.Level+" · "+issue.Path+" · "+message).TextColor(c.Theme().Warning)
		}
		if p.loaded && len(p.result.issues) == 0 {
			ui.Text(c, "No validation issues found.")
		}
		if p.message != "" {
			ui.Text(c, p.message).Role(ui.RoleStatus)
		}
	})
}
