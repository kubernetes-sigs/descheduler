# Descheduler Plugin: Example Implementation

This directory provides an example plugin for the Kubernetes Descheduler,
demonstrating how to evict pods based on custom criteria. The plugin targets
pods based on:

* **Name Regex:** Pods matching a specified regular expression.
* **Age:** Pods older than a defined duration.
* **Namespace:** Pods within or outside a given list of namespaces (inclusion
  or exclusion).

## Building and Integrating the Plugin

To incorporate this plugin into your Descheduler build, you must register it
within the Descheduler's plugin registry. Follow these steps:

1.  **Register the Plugin:**
    * Modify the `pkg/descheduler/setupplugins.go` file.
    * Add the following registration line to the end of the
      `RegisterDefaultPlugins()` function:

    ```go
    pluginregistry.Register(
      example.PluginName,
      example.New,
      &example.Example{},
      &example.ExampleArgs{},
      example.ValidateExampleArgs,
      example.SetDefaults_Example,
      registry,
    )
    ```

2.  **Generate Code:**
    * If you modify the plugin's code, execute `make gen` before rebuilding the
      Descheduler. This ensures generated code is up-to-date.

3.  **Rebuild the Descheduler:**
    * Build the descheduler with your changes.

## Plugin Configuration

Configure the plugin's behavior using the Descheduler's policy configuration.
Here's an example:

```yaml
apiVersion: descheduler/v1alpha2
kind: DeschedulerPolicy
profiles:
- name: LifecycleAndUtilization
  plugins:
    deschedule:
      enabled:
        - Example
  pluginConfig:
  - name: Example
    args:
      regex: ^descheduler-test.*$
      maxAge: 3m
      namespaces:
        include:
        - default
```

## Explanation

- `regex: ^descheduler-test.*$`: Evicts pods whose names match the regular
  expression `^descheduler-test.*$`.
- `maxAge: 3m`: Evicts pods older than 3 minutes.
- `namespaces.include: - default`: Evicts pods within the default namespace.

This configuration will cause the plugin to evict pods that meet all three
criteria: matching the `regex`, exceeding the `maxAge`, and residing in the
specified namespace.

## Notes

- This plugin is configured through the `ExampleArgs` struct, which defines the
  plugin's parameters.
- Plugins must implement a function to validate and another to set the default
  values for their `Args` struct.
- The fields in the `ExampleArgs` struct reflect directly into the
  `DeschedulerPolicy` configuration.
- Plugins must comply with the `DeschedulePlugin` interface to be registered
  with the Descheduler.
- The main functionality of the plugin is implemented in the `Deschedule()`
  method, which is called by the Descheduler when the plugin is executed.
- A good amount of descheduling logic can be achieved by means of filters.
- Whenever a change in the Plugin's configuration is made the developer should
  regenerate the code by running `make gen`.
