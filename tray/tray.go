package tray

import (
	"sync"

	"github.com/getlantern/systray"
	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/server"
)

var (
	initOnce sync.Once

	srvr *server.Server
)


// Init starts running the system tray. It's required before using this package
func Init(serv *server.Server) {
	initOnce.Do(func() {
		log.Info("Starting system tray")

		srvr = serv

		ready := make(chan interface{})

		go systray.Run(func() {
			// TODO: icon instead of title
			systray.SetTitle("ST")
			systray.SetTooltip("Use servicetray from the cmdline to manage services")

			log.Debug("Done setting up tray")
			close(ready)
		})

		<-ready
	})
}

// Quit shuts down the tray and cleans up
func Quit() {
	systray.Quit()
	srvr = nil
}
