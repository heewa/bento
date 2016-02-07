package server

import (
	"fmt"
	"reflect"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/config"
	"github.com/heewa/servicetray/service"
)

// LoadServicesArgs are arguments to the LoadServices cmd
type LoadServicesArgs struct {
	ServiceFilePath string
}

// LoadServicesResponse is the response from the LoadServices cmd
type LoadServicesResponse struct {
	NewServices        []service.Info
	UpdatedServices    []service.Info
	DeprecatedServices []service.Info
	RemovedServices    []string
}

// LoadServices will start a new, temp service
func (s *Server) LoadServices(args LoadServicesArgs, reply *LoadServicesResponse) error {
	log.Info("Load services", "file", args.ServiceFilePath)
	confs, err := config.LoadServiceFile(args.ServiceFilePath)
	if err != nil {
		return err
	}

	confsToLoad := make(map[string]*config.Service)

	log.Debug("Loaded service confs", "num", len(confs))
	for _, conf := range confs {
		confsToLoad[conf.Name] = &conf

		if srvc := s.getService(conf.Name); srvc != nil && !reflect.DeepEqual(srvc.Conf, conf) {
			log.Debug("Updating an existing service", "conf", conf)

			if srvc.Conf.Temp && srvc.Running() {
				// If conf is adding back a service that had earlier been
				// marked as temp because of a removal from conf, and is now
				// being restored, un-temp-ify it.
				if srvc.Conf.EqualIgnoringTemp(&conf) {
					if !s.changeServicePermanence(srvc.Conf.Name, false, 0) {
						return fmt.Errorf("Failed to remove temporary status of a now-permanent service (%s)", srvc.Conf.Name)
					}
				} else {
					return fmt.Errorf("Cannot add a service that has the same name as a running temp service (%s)", conf.Name)
				}
			} else {
				// Replacing a changed service
				newSrvc, err := service.New(conf)
				if err != nil {
					return fmt.Errorf("Failed to create a changed service (%s): %v", conf.Name, err)
				}

				if err := s.addService(newSrvc, true); err != nil {
					return fmt.Errorf("Failed to add back a changed service (%s): %v", conf.Name, err)
				}

				reply.UpdatedServices = append(reply.UpdatedServices, newSrvc.Info())
			}
		} else if srvc == nil {
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
