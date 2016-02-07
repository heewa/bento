package tray

import (
	"fmt"
	"sync"

	"github.com/getlantern/systray"
	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

const (
	mainTitle   = "🍱"
	mainTooltip = "Use servicetray from the cmdline to manage services"

	quitTitle   = "Quit ServiceTray"
	quitTooltip = "Beware: quitting will stop all services!"
)

var (
	initOnce sync.Once

	srvr *server.Server

	itemLock     sync.RWMutex
	serviceItems []ServiceItem
	quitItem     *systray.MenuItem
	deadItems    []*systray.MenuItem
)

// Init starts running the system tray. It's required before using this package
func Init() {
	initOnce.Do(func() {
		log.Info("Starting system tray")

		srvr = nil

		ready := make(chan interface{})

		go systray.Run(func() {
			// TODO: icon instead of title
			systray.SetTitle(mainTitle)
			systray.SetTooltip(mainTooltip)

			// TODO: revive without dead items

			itemLock.Lock()
			defer itemLock.Unlock()

			quitItem = systray.AddMenuItem(quitTitle, quitTooltip)
			go handleClick(quitItem.ClickedCh, 0)

			log.Debug("Done setting up tray")
			close(ready)
		})

		<-ready
	})
}

// SetServer gives the Tray UI a reference to the server, and some channels for
// the server to communicate with it. This is separated from Init so the UI can
// start without a server, in case there's an error starting the server, the UI
// will be able to display an error, instead of just not being initialized.
func SetServer(serv *server.Server, serviceUpdates <-chan service.Info) error {
	if srvr != nil {
		return fmt.Errorf("Multiple calls to SetServer")
	}

	srvr = serv

	// Watch for service changes
	go func() {
		for {
			info := <-serviceUpdates
			if info.Dead {
				RemoveService(info.Name)
			} else {
				SetService(info)
			}
		}
	}()

	return nil
}

// Quit shuts down the tray and cleans up
func Quit() {
	log.Debug("Stopping system tray")
	systray.Quit()

	log.Debug("Clearing tray vars")
	itemLock.Lock()
	defer itemLock.Unlock()

	serviceItems = nil
	quitItem = nil
	deadItems = nil
	srvr = nil
}

// SetService adds or updates a service to the tray
func SetService(info service.Info) {
	itemLock.Lock()
	defer itemLock.Unlock()

	// See if it exists already to update
	for _, item := range serviceItems {
		if item.info.Name == info.Name {
			item.Set(info)
			return
		}
	}

	// Use Quit's slot as a new item and shift Quit down
	var item ServiceItem
	item.menu, quitItem = quitItem, nil

	item.Set(info)
	serviceItems = append(serviceItems, item)

	// If there are dead slots, use one for Quit
	if len(deadItems) > 0 {
		quitItem, deadItems = deadItems[0], deadItems[1:]
		quitItem.SetTitle(quitTitle)
		quitItem.SetTooltip(quitTooltip)
	} else {
		quitItem = systray.AddMenuItem(quitTitle, quitTooltip)
		go handleClick(quitItem.ClickedCh, len(serviceItems))
	}
}

// RemoveService removes an item from the tray
func RemoveService(name string) {
	// The system tray implementation doesn't support removing, so just clean
	// it out and swap with the end of the list
	itemLock.Lock()
	defer itemLock.Unlock()

	// Find the item
	index := -1
	for i := 0; index == -1 && i < len(serviceItems); i++ {
		if serviceItems[i].info.Name == name {
			index = i
		}
	}

	// Nothing to remove
	if index == -1 {
		return
	}

	// Move the last alive item to this position, and move Quit to the last
	// item. Like:
	//     Service A
	//     Dead Service <---
	//     Service B       |
	//     Service C -------  <-
	//     Quit ----------------
	lastIndex := len(serviceItems) - 1
	if index < lastIndex {
		serviceItems[index].Set(serviceItems[lastIndex].info)
	}

	// Clear and add current Quit to dead items
	quitItem.SetTitle("")
	quitItem.SetTooltip("")
	deadItems = append([]*systray.MenuItem{quitItem}, deadItems...)

	// Use lastIndex for Quit
	quitItem = serviceItems[lastIndex].menu
	quitItem.SetTitle(quitTitle)
	quitItem.SetTooltip(quitTooltip)

	// Remove last service item from slice
	serviceItems = serviceItems[:lastIndex]
}

// Since items change roles over time, look up logical item at each click
func handleClick(click <-chan interface{}, index int) {
	for {
		<-click
		log.Debug("Click on menu item", "index", index)

		func() {
			itemLock.RLock()
			defer itemLock.RUnlock()

			if index < len(serviceItems) {
				item := serviceItems[index]

				if item.menu.Checked() {
					reply := server.StopResponse{}
					if err := srvr.Stop(server.StopArgs{item.info.Name}, &reply); err != nil {
						log.Warn("Failed to stop service", "service", item.info.Name, "err", err)
					}
					go SetService(reply.Info)
				} else {
					reply := server.StartResponse{}
					if err := srvr.Start(server.StartArgs{item.info.Name}, &reply); err != nil {
						log.Warn("Failed to start service", "service", item.info.Name, "err", err)
					}
					go SetService(reply.Info)
				}
			} else if index == len(serviceItems) {
				// Quit
				var nothing bool
				if err := srvr.Exit(nothing, &nothing); err != nil {
					// Log, and communicate with user through menu
					log.Error("Failed to exit server", "err", err)
					quitItem.SetTitle("Quit -- ERR, use cmdline")
					quitItem.SetTooltip(err.Error())
				}
			} // else it's a dead item, ignore
		}()
	}
}
