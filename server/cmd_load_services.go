package server

import (
	"fmt"
	"reflect"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/config"
	"github.com/heewa/servicetray/service"
)

// LoadServicesArgs -
type LoadServicesArgs struct {
	ServiceFilePath string
}

// LoadServicesResponse -
type LoadServicesResponse struct {
	NewServices        []service.Info
	UpdatedServices    []service.Info
	DeprecatedServices []service.Info
	RemovedServices    []string
}

// LoadServices will start a new, temp service
func (s *Server) LoadServices(args LoadServicesArgs, reply *LoadServicesResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	log.Info("Load services", "file", args.ServiceFilePath)
	confs, err := config.LoadServiceFile(args.ServiceFilePath)
	if err != nil {
		return err
	}

	confsToLoad := make(map[string]*config.Service)

	log.Debug("Loaded service confs", "num", len(confs))
	for _, conf := range confs {
		confsToLoad[conf.Name] = &conf

		if srvc := s.getService(conf.Name); srvc == nil {
			log.Debug("Adding a new service", "conf", conf)

			newSrvc, err := service.New(conf)
			if err != nil {
				return fmt.Errorf("Failed to create a new service (%s): %v", conf.Name, err)
			}

			if err := s.addService(newSrvc, false); err != nil {
				// Weird, that shouldn't happen.
				return fmt.Errorf("Failed to add what looks like a new service (%s): %v", conf.Name, err)
			}

			reply.NewServices = append(reply.NewServices, newSrvc.Info())
		} else if reflect.DeepEqual(srvc.Conf, conf) {
			// Unmodified service, ignore
		} else if !srvc.Running() {
			// Since it's not running, ignore issue of safe changes, and just
			// replace it.
			log.Debug("Replacing a changed service", "current", srvc.Conf, "new", conf)

			newSrvc, err := service.New(conf)
			if err != nil {
				return fmt.Errorf("Failed to create a changed service (%s): %v", conf.Name, err)
			}

			if err := s.addService(newSrvc, true); err != nil {
				return fmt.Errorf("Failed to add back a changed service (%s): %v", conf.Name, err)
			}

			reply.UpdatedServices = append(reply.UpdatedServices, newSrvc.Info())
		} else if srvc.Conf.EqualIgnoringSafeFields(&conf) {
			log.Debug("Updating an running service with safe changes", "curent", srvc.Conf, "new", conf)

			// If conf is adding back a service that had earlier been
			// marked as temp because of a removal from conf, and is now
			// being restored, un-temp-ify it.
			if srvc.Conf.Temp && !conf.Temp && !s.changeServicePermanence(srvc.Conf.Name, false, 0) {
				return fmt.Errorf("Failed to remove temporary status of a now-permanent service (%s)", srvc.Conf.Name)
			}

			// Auto-start is safe to just set or clean on a conf of a service
			// that's already running
			srvc.Conf.AutoStart = conf.AutoStart

			// Changing restart-on-exit requires some work, though
			if !srvc.Conf.RestartOnExit && conf.RestartOnExit {
				s.addServiceToRestartWatch(srvc)
				srvc.Conf.RestartOnExit = true
			} else if srvc.Conf.RestartOnExit && !conf.RestartOnExit {
				s.removeServiceFromRestartWatch(srvc.Conf.Name)
				srvc.Conf.RestartOnExit = false
			}

			// To be sure we didn't forget to add logic to set a safe field,
			// check that all changes were made.
			if !reflect.DeepEqual(srvc.Conf, conf) {
				return fmt.Errorf("Failed to fully apply conf changes to service (%s)", srvc.Conf.Name)
			}

			reply.UpdatedServices = append(reply.UpdatedServices, srvc.Info())
		} else {
			return fmt.Errorf("Cannot apply these changes to a running service (%s)", conf.Name)
		}
	}

	// Check for removed services
	for _, srvc := range s.listServices() {
		if conf := confsToLoad[srvc.Conf.Name]; conf == nil {
			// If it's not running, just remove it
			if !srvc.Running() {
				log.Info("Removing service that's no longer in conf", "name", srvc.Conf.Name)
				s.removeService(srvc.Conf.Name)
				reply.RemovedServices = append(reply.RemovedServices, srvc.Conf.Name)
			} else {
				// Since it's still running, mark it as temporary with an immediate clean up
				log.Info("Service that's no longer in conf is running, marking as temp for removal after exit", "name", srvc.Conf.Name)
				if !s.changeServicePermanence(srvc.Conf.Name, true, 0) {
					return fmt.Errorf("Failed to set a removed, but still running servicey (%s) as temporary for cleanup when it exits", srvc.Conf.Name)
				}
				reply.DeprecatedServices = append(reply.DeprecatedServices, srvc.Info())
			}
		}
	}

	return nil
}
