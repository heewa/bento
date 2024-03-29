package tray

import (
	"fmt"
	"sync"

	"github.com/getlantern/systray"
	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/server"
	"github.com/heewa/bento/service"
)

const (
	activeIcon  = "🍱"
	idleIcon    = "🍚"
	mainTooltip = "Use bento from the cmdline to manage services"

	quitTitle   = "Quit Bento"
	quitTooltip = "Beware: quitting will stop all services!"
)

var (
	initOnce sync.Once

	srvr *server.Server

	itemLock     sync.RWMutex
	errorItem    *systray.MenuItem
	serviceItems []*ServiceItem
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
			systray.SetTitle(idleIcon)
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

	currentIcon := idleIcon

	// Watch for service changes
	go func() {
		for {
			info, ok := <-serviceUpdates
			if !ok {
				return
			}

			if info.Dead {
				RemoveService(info.Name)
			} else {
				SetService(info)
			}

			// If any services are running, set title to the active icon,
			// otherwise the idle one.
			func() {
				// Shortcut if this updated service is running
				if info.Running {
					if currentIcon != activeIcon {
						currentIcon = activeIcon
						systray.SetTitle(activeIcon)
					}
					return
				}

				itemLock.RLock()
				defer itemLock.RUnlock()

				for _, item := range serviceItems {
					if item.info.Running {
						if currentIcon != activeIcon {
							currentIcon = activeIcon
							systray.SetTitle(activeIcon)
						}
						return
					}
				}

				// None of them were running; set to idle
				if currentIcon != idleIcon {
					currentIcon = idleIcon
					systray.SetTitle(idleIcon)
				}
			}()
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

	errorItem = nil
	serviceItems = nil
	quitItem = nil
	deadItems = nil

	srvr = nil

	log.Info("Tray is done")
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
	serviceItems = append(serviceItems, &item)

	// If there are dead slots, use one for Quit
	if len(deadItems) > 0 {
		quitItem, deadItems = deadItems[0], deadItems[1:]
		quitItem.SetTitle(quitTitle)
		quitItem.SetTooltip(quitTooltip)
	} else {
		quitItem = systray.AddMenuItem(quitTitle, quitTooltip)

		index := len(serviceItems)
		if errorItem != nil {
			index++
		}
		go handleClick(quitItem.ClickedCh, index)
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
	quitItem.Uncheck()

	// Remove last service item from slice
	serviceItems = serviceItems[:lastIndex]
}

// Since items change roles over time, look up logical item at each click
func handleClick(click <-chan interface{}, index int) {
	for {
		_, ok := <-click
		if !ok {
			return
		}
		log.Debug("Click on menu item", "index", index)

		func() {
			itemLock.RLock()
			defer itemLock.RUnlock()

			if index == 0 && errorItem != nil {
				// Click on error, need to clear it, but that thing needs the
				// lock we're holding, so go it in a goroutine that'll unblock
				// after we're done.
				go ClearError()
				return
			}

			// To make indexing into serviceItems easier, handle conditional
			// errorItem by fixing up index
			index := index
			if errorItem != nil {
				index--
			}

			if index < len(serviceItems) {
				item := serviceItems[index]

				if item.menu.Checked() {
					if err := srvr.Stop(server.StopArgs{Name: item.info.Name}, nil); err != nil {
						log.Warn("Failed to stop service", "service", item.info.Name, "err", err)
					}
				} else {
					if err := srvr.Start(server.StartArgs{Name: item.info.Name}, nil); err != nil {
						log.Warn("Failed to start service", "service", item.info.Name, "err", err)
					}
				}
			} else if index == len(serviceItems) {
				log.Debug("Clicked on quit")

				if srvr == nil {
					// Never had a chance to start server, so just quit tray
					go Quit()
				} else {
					var nothing bool
					if err := srvr.Exit(nothing, &nothing); err != nil {
						log.Error("Failed to exit server", "err", err)

						// Since server won't be exitting and quitting the
						// tray, do that ourselves
						go Quit()
					}
				}
			} // else it's a dead item, ignore
		}()
	}
}
