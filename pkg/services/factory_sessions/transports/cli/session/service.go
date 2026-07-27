package session

import "fmt"

// Service exposes Factory Session CLI command operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	Show(ShowConfig) error
	Pause(LifecycleControlConfig) error
	Resume(LifecycleControlConfig) error
	ListDispatches(DispatchesConfig) error
	Create(CreateConfig) error
	Delete(DeleteConfig) error
}

// Operations carries the accepted per-command operations used to build Service.
type Operations struct {
	List           func(ListConfig) error
	Show           func(ShowConfig) error
	Pause          func(LifecycleControlConfig) error
	Resume         func(LifecycleControlConfig) error
	ListDispatches func(DispatchesConfig) error
	Create         func(CreateConfig) error
	Delete         func(DeleteConfig) error
}

type service struct {
	list           func(ListConfig) error
	show           func(ShowConfig) error
	pause          func(LifecycleControlConfig) error
	resume         func(LifecycleControlConfig) error
	listDispatches func(DispatchesConfig) error
	create         func(CreateConfig) error
	delete         func(DeleteConfig) error
}

// Bind constructs the typed Sessions CLI service from injected operations.
func Bind(ops Operations) Service {
	return &service{
		list:           ops.List,
		show:           ops.Show,
		pause:          ops.Pause,
		resume:         ops.Resume,
		listDispatches: ops.ListDispatches,
		create:         ops.Create,
		delete:         ops.Delete,
	}
}

func (service *service) List(cfg ListConfig) error {
	if service == nil || service.list == nil {
		return fmt.Errorf("session list service is required")
	}
	return service.list(cfg)
}

func (service *service) Show(cfg ShowConfig) error {
	if service == nil || service.show == nil {
		return fmt.Errorf("session show service is required")
	}
	return service.show(cfg)
}

func (service *service) Pause(cfg LifecycleControlConfig) error {
	if service == nil || service.pause == nil {
		return fmt.Errorf("session pause service is required")
	}
	return service.pause(cfg)
}

func (service *service) Resume(cfg LifecycleControlConfig) error {
	if service == nil || service.resume == nil {
		return fmt.Errorf("session resume service is required")
	}
	return service.resume(cfg)
}

func (service *service) ListDispatches(cfg DispatchesConfig) error {
	if service == nil || service.listDispatches == nil {
		return fmt.Errorf("session dispatches service is required")
	}
	return service.listDispatches(cfg)
}

func (service *service) Create(cfg CreateConfig) error {
	if service == nil || service.create == nil {
		return fmt.Errorf("session create service is required")
	}
	return service.create(cfg)
}

func (service *service) Delete(cfg DeleteConfig) error {
	if service == nil || service.delete == nil {
		return fmt.Errorf("session delete service is required")
	}
	return service.delete(cfg)
}
