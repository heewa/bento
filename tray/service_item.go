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
	if info.Running || info.Succeeded || info.Pid == 0 {
		item.menu.SetTitle(info.Name)
	} else {
		// If it ran and failed, mention that in title
		item.menu.SetTitle(fmt.Sprintf("%s <failed>", info.Name))
	}

	if info.Running && !item.info.Running {
		item.menu.Check()
	} else if !info.Running && item.info.Running {
		item.menu.Uncheck()
	}

	if len(info.Tail) > 0 {
		item.menu.SetTooltip(strings.Join(info.Tail, "\n"))
	} else {
		item.menu.SetTooltip(info.PlainString())
	}

	item.info = info
}
