package app

import (
	"log"

	"github.com/egoist/mygo"

	"ringo/internal/telemetry"
)

// startTelemetry has the player tell the listener's own collector what
// plays, beside Apple, once the endpoint the settings keep is set up.
func (a *app) startTelemetry() {
	a.plays = &telemetry.Plays{Version: mygo.App.Version()}
	a.pl.Heard = a.plays.Played
	endpoint, headers := a.settings.TelemetryEndpoint, a.settings.TelemetryHeaders
	if endpoint == "" {
		return
	}
	go func() {
		if err := a.plays.Configure(endpoint, headers); err != nil {
			// The error never holds the headers, which may hold a key.
			log.Printf("telemetry: %v", err)
		}
	}()
}

// keepTelemetry keeps the endpoint and headers and sends there from now on;
// one that can't be used is said, and nothing is sent until it's put right.
func (a *app) keepTelemetry(endpoint, headers string) {
	if endpoint == a.settings.TelemetryEndpoint && headers == a.settings.TelemetryHeaders {
		return
	}
	a.settings.TelemetryEndpoint, a.settings.TelemetryHeaders = endpoint, headers
	a.changed()
	go func() {
		err := a.plays.Configure(endpoint, headers)
		a.post(func() {
			if err != nil {
				a.toast = Tf("Couldn't keep the setting: %s", err.Error())
			}
		})
	}()
}
