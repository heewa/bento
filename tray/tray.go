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

// ServiceItem is a menu item for a Service
type ServiceItem struct {
	menu *systray.MenuItem
	info service.Info
}

// Set updates with Service info
func (item *ServiceItem) Set(info service.Info) {
	item.info = info

	// If it ran and failed, mention that in title
	if !info.Running && info.Pid != 0 && !info.Succeeded {
		item.menu.SetTitle(fmt.Sprintf("%s <failed>", info.Name))
	} else {
		item.menu.SetTitle(info.Name)
	}

	if info.Running {
		item.menu.Check()
	} else {
		item.menu.Uncheck()
	}

	item.menu.SetTooltip(info.String())
}

// Init starts running the system tray. It's required before using this package
func Init(serv *server.Server, serviceUpdates <-chan service.Info) {
	initOnce.Do(func() {
		log.Info("Starting system tray")

		srvr = serv

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

		// Watch for service changes
		go func() {
			for {
				SetService(<-serviceUpdates)
			}
		}()
	})
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
	// TODO: implement this by swapping items around
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
