# Fix for Issue #1695: Cyclic Eviction of Large Pods in LowNodeUtilization

## Problem Description

**Issue**: [GitHub Issue #1695](https://github.com/kubernetes-sigs/descheduler/issues/1695)

When using the `LowNodeUtilization` strategy, large pods that overload any node were being evicted every 5 minutes in a continuous loop. This happened because:

1. **Large pods exceed node capacity** - The pods are so large that they cannot fit on any node without exceeding the target thresholds
2. **No eviction filtering** - The `LowNodeUtilization` strategy didn't have the strict eviction mode that was already available in `HighNodeUtilization`
3. **Continuous cycle** - The descheduler kept trying to evict the same large pods repeatedly, causing unnecessary disruption

### Example Scenario
```
Cluster: 2 nodes with 1 CPU, 1GB RAM each
Large Pod: Uses 800m CPU, 800Mi RAM (no resource requests defined)
Small Pods: Use 200m CPU, 200Mi RAM each (with resource requests)

Problem: Large pod gets evicted → rescheduled → overloads new node → evicted again → cycle repeats
```

## Solution

The fix adds the `EvictionModes` feature to `LowNodeUtilization`, specifically the `OnlyThresholdingResources` mode that was already available in `HighNodeUtilization`.

### What Changed

1. **Added `EvictionModes` field to `LowNodeUtilizationArgs`**
   - Allows users to specify eviction filtering modes
   - Default behavior remains unchanged (backward compatible)

2. **Added validation for `EvictionModes`**
   - Ensures only valid eviction modes are used
   - Validates the `OnlyThresholdingResources` mode

3. **Enhanced pod filtering logic**
   - When `OnlyThresholdingResources` mode is enabled, only pods with resource requests for the thresholding resources are considered for eviction
   - This prevents eviction of pods that don't actually request the resources being used for threshold calculations

### Code Changes

#### 1. Types (`pkg/framework/plugins/nodeutilization/types.go`)
```go
type LowNodeUtilizationArgs struct {
    // ... existing fields ...
    
    // EvictionModes is a set of modes to be taken into account when the
    // descheduler evicts pods. For example the mode
    // `OnlyThresholdingResources` can be used to make sure the descheduler
    // only evicts pods that have resource requests for the resources that
    // are being used for threshold calculations.
    EvictionModes []EvictionMode `json:"evictionModes,omitempty"`
}
```

#### 2. Validation (`pkg/framework/plugins/nodeutilization/validation.go`)
```go
func ValidateLowNodeUtilizationArgs(obj runtime.Object) error {
    args := obj.(*LowNodeUtilizationArgs)
    // ... existing validation ...
    
    // make sure we know about the eviction modes defined by the user.
    if err := validateEvictionModes(args.EvictionModes); err != nil {
        return err
    }
    
    // ... rest of validation ...
}
```

#### 3. Implementation (`pkg/framework/plugins/nodeutilization/lownodeutilization.go`)
```go
// Enhanced pod filter creation with eviction modes support
filters := []podutil.FilterFunc{handle.Evictor().Filter}
if slices.Contains(args.EvictionModes, EvictionModeOnlyThresholdingResources) {
    filters = append(filters, withResourceRequestForAny(args.Thresholds))
}

podFilter, err := podutil.NewOptions().WithFilter(podutil.CombineFilters(filters...)).BuildFilterFunc()
```

## Usage Examples

### 1. Default Behavior (No Change)
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "LowNodeUtilization":
    enabled: true
    params:
      nodeResourceUtilizationThresholds:
        thresholds:
          "cpu" : 20
          "memory": 20
        targetThresholds:
          "cpu" : 70
          "memory": 70
```
**Result**: All pods are considered for eviction (original behavior)

### 2. With EvictionModes Fix
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "LowNodeUtilization":
    enabled: true
    params:
      nodeResourceUtilizationThresholds:
        thresholds:
          "cpu" : 20
          "memory": 20
        targetThresholds:
          "cpu" : 70
          "memory": 70
        evictionModes:
          - "OnlyThresholdingResources"
```
**Result**: Only pods with CPU or memory resource requests are considered for eviction

### 3. Complete Example with All Features
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "LowNodeUtilization":
    enabled: true
    params:
      nodeResourceUtilizationThresholds:
        thresholds:
          "cpu" : 20
          "memory": 20
          "pods": 20
        targetThresholds:
          "cpu" : 70
          "memory": 70
          "pods": 70
        evictionModes:
          - "OnlyThresholdingResources"
        numberOfNodes: 3
        evictableNamespaces:
          exclude:
            - "kube-system"
            - "default"
```

## Testing

The fix includes comprehensive tests that verify:

1. **Issue Reproduction**: Tests confirm that large pods without resource requests get evicted cyclically without the fix
2. **Fix Verification**: Tests confirm that large pods without resource requests are filtered out when using `OnlyThresholdingResources` mode
3. **Backward Compatibility**: Tests confirm that existing behavior is preserved when `EvictionModes` is not specified
4. **Selective Eviction**: Tests confirm that pods with resource requests are still evicted when appropriate

### Running Tests
```bash
# Run the specific tests for this fix
go test -v -run "TestLowNodeUtilizationCyclicEviction"

# Run all nodeutilization tests
go test -v ./pkg/framework/plugins/nodeutilization/
```

## Migration Guide

### For Existing Users
- **No action required** - The fix is backward compatible
- Existing configurations will continue to work exactly as before
- To enable the fix, add `evictionModes: ["OnlyThresholdingResources"]` to your configuration

### For New Users
- Consider using `evictionModes: ["OnlyThresholdingResources"]` to prevent cyclic eviction issues
- This is especially important if you have pods without resource requests that might be large

### Recommended Configuration
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "LowNodeUtilization":
    enabled: true
    params:
      nodeResourceUtilizationThresholds:
        thresholds:
          "cpu" : 20
          "memory": 20
        targetThresholds:
          "cpu" : 70
          "memory": 70
        evictionModes:
          - "OnlyThresholdingResources"  # Recommended to prevent cyclic eviction
```

## Benefits

1. **Prevents Cyclic Eviction**: Large pods without resource requests are no longer evicted repeatedly
2. **Reduces Cluster Disruption**: Eliminates unnecessary pod evictions and rescheduling
3. **Maintains Functionality**: Pods with proper resource requests are still evicted when appropriate
4. **Backward Compatible**: Existing configurations continue to work without changes
5. **Consistent with HighNodeUtilization**: Both strategies now have the same eviction filtering capabilities

## Related Issues

- [Issue #1695](https://github.com/kubernetes-sigs/descheduler/issues/1695) - Original issue report
- [HighNodeUtilization EvictionModes](https://github.com/kubernetes-sigs/descheduler/pull/XXXX) - Similar feature for HighNodeUtilization

## Contributing

This fix follows the existing code patterns and conventions in the descheduler project. The implementation:

- Uses the same `EvictionMode` type as `HighNodeUtilization`
- Follows the same validation patterns
- Includes comprehensive tests
- Maintains backward compatibility
- Adds proper documentation 