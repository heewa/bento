package tray

import (
	"fmt"

	"github.com/getlantern/systray"
	log "github.com/inconshreveable/log15"
)

// Error is a an error that's meant for display as a menu item in the tray
type Error struct {
	title   string
	tooltip string
}

// NewError creates a new UI error.
func NewError(title string, err error) *Error {
	return &Error{
		title:   fmt.Sprintf("[ERR] %s -- consult cmdline for more info", title),
		tooltip: err.Error(),
	}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s -- %s", e.title, e.tooltip)
}

// SetError creates or updates a menu item at the top with the error txt
func SetError(err *Error) {
	if err == nil {
		ClearError()
		return
	}
	log.Error("Setting menu error", "err", err)

	itemLock.Lock()
	defer itemLock.Unlock()

	if errorItem == nil {
		// Shuffle items down to use whatever's at the top as errorItem, starting
		// at the bottom and working up

		// If there are dead items, use one for quit, otherwise make a new quit
		var newQuit *systray.MenuItem
		if len(deadItems) > 0 {
			newQuit, deadItems = deadItems[0], deadItems[1:]
			newQuit.SetTooltip(quitTooltip)
		} else {
			newQuit = systray.AddMenuItem(quitTitle, quitTooltip)
			go handleClick(newQuit.ClickedCh, len(serviceItems)+1)
		}

		// If there are service items, swap the first one with old quit item
		if len(serviceItems) > 0 {
			quitItem, serviceItems[0].menu = serviceItems[0].menu, quitItem
			serviceItems[0].Set(serviceItems[0].info)
		}

		// The leftover goes to errorItem, and fixup quitItem
		errorItem, quitItem = quitItem, newQuit
	}

	errorItem.SetTitle(err.title)
	errorItem.SetTooltip(err.tooltip)
}

// ClearError clears the first item in the tray if it's an error by shuffling
// items around, similarly to RemoveService()
func ClearError() {
	log.Debug("Clearing error in menu")

	itemLock.Lock()
	defer itemLock.Unlock()

	if errorItem == nil {
		return
	}

	var newDead *systray.MenuItem

	lastIndex := len(serviceItems) - 1
	if lastIndex < 0 {
		// Since there are no service items, just shift up quit
		// over error, and quit becomes dead
		errorItem, quitItem, newDead = nil, errorItem, quitItem
	} else {
		// Similar to other case, but one more item to shift
		errorItem, serviceItems[lastIndex].menu, quitItem, newDead = nil, errorItem, serviceItems[lastIndex].menu, quitItem

		serviceItems[lastIndex].Set(serviceItems[lastIndex].info)
	}

	// Fix up quit & dead's texts
	quitItem.SetTitle(quitTitle)
	quitItem.SetTooltip(quitTooltip)
	newDead.SetTitle("")
	newDead.SetTooltip("")

	// Push newly created dead item to front of dead list
	deadItems = append([]*systray.MenuItem{newDead}, deadItems...)
}
