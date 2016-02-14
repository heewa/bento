package tray

import (
	"fmt"
	"strings"

	"github.com/getlantern/systray"

	"github.com/heewa/bento/service"
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

	if len(info.Tail) > 0 {
		item.menu.SetTooltip(strings.Join(info.Tail, "\n"))
	} else {
		item.menu.SetTooltip(info.PlainString())
	}
}
