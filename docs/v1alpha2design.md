# Descheduler v1alpha2 Design Proposal

## Summary
This document is intended to provide a design for Descheduler v1apha2 Policy Config API. This API will aim at let Descheduler more extendable and users can more easy to implement custom strategy.
(Not include for informer yet, I plan to add support informer to v1beta1)

## Motivation
Many descheduler strategys are being added to the Descheduler. They keep making the code larger and the logic more complex. A more complex scheduler is harder to maintain, its bugs are harder to find and fix, and those users running a custom descheduler have a hard time catching up and integrating new changes.  

## Goals
* Make descheduler more extendable,

## Proposal

### API
The current Policy API is defined in pkg/api/v1alpha1/types.go. The new Descheduler Policy API would be based on the existing API with the following changes:
```go
type DeschedulerPolicy struct {
  metav1.TypeMeta `json:”,inline”`

  Profiles []DeschedulerProfile `json:”profiles”`
  # OutofTreeRegistry Registry `json:"outofTreeRegistry"`
  ...
}

type DeschedulerProfile struct {
    // ... other fields
    Plugins      Plugins
    PluginConfig []PluginConfig
}

type PluginConfig struct {

}
```

new command
```
type Option func(Registry) error

func WithPlugin(name string, factory PluginFactory) Option {
	return func(registry Registry) error {
		return Register(name, factory)
	}
}

// NewSchedulerCommand creates a *cobra.Command object with default parameters and registryOptions
func NewDeschedulerCommand(registryOptions ...Option) *cobra.Command {
        ...
			RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, registryOptions...)
		},
```
### main function 
```go
func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string, stopChannel chan struct{}) error {
	...
    registry := NewRegistry()
    // merge outof tree plugins
    registry.merge(deschedulerPolicy.OutofTreeRegistry)
    registry = merge(registry, options)
    for name, pluginfactory := range registry {
        log("plugin name is ...", name)
        pluginfactory.Run(ctx, rs.Client, strategy, nodes, podEvictor, getPodsAssignedToNode)
    }
}

func (r Registry) merge(outoftreeregistry Registry) {
    for name, factory := range outoftreeregistry {
		if err := r.Register(name, factory); err != nil {
			return err
		}
	}
	return nil
}

func (r Registry) Register(name string, factory PluginFactory) error {
	if _, ok := r[name]; ok {
		return fmt.Errorf("a plugin named %v already exists", name)
	}
	r[name] = factory
	return nil
}

func (r Registry) Unregister(name string) error {
	if _, ok := r[name]; !ok {
		return fmt.Errorf("no plugin named %v exists", name)
	}
	delete(r, name)
	return nil
}
```
## Plugin API
There are two steps to the plugin API. First, plugins must register and get configured, then they use the extension point interfaces. Extension point interfaces have the following form.
Define interface.go as :
```go
type Plugin interface {
	Name() string
	// run is for peroid mode run function
    Run(ctx, rs.Client, strategy, nodes, podEvictor, getPodsAssignedToNode)
}

```
### Plugin Registration
Each plugin must define a constructor and add it to the hard-coded registry.
Example:
```go
type PluginFactory = func(configuration runtime.Objec) (Plugin, error)

type Registry map[string]PluginFactory

func NewRegistry() Registry {
    return Registry{
        duplicates.Name: duplicates.New(),
        failedpods.Name: failedpods.New(),
        // New plugins are registered here.
    }
}
```


This is an example for duplicates strategy plugin
```go
const Name = "duplicates"

type Dplicates struct{}

// Name returns name of the plugin.
func (d *duplicates) Name() string {
	return Name
}

func New(_ runtime.Object) (Plugin, error) {
	return &Dplicates{}, nil
}

func (d *duplicates) Run(ctx, rs.Client, strategy, nodes, podEvictor, getPodsAssignedToNode) {
	...
}
```

## how to write a apps for extend descheduler
example as below:
```
unc main() {
	command := app.NewDeschedulerCommand(
		app.WithPlugin(nodestate.Name, nodestate.New),
	)

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
```
