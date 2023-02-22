package hooksutil

import (
	"reflect"

	"github.com/sirupsen/logrus"
	"isc.org/stork/hooks"
)

// Function that calls a specific callout in the callout carrier.
type Caller = func(carrier hooks.CalloutCarrier)

// Manages all loaded hooks and allows to call their callouts.
// The caller may choose different calling strategies.
type HookExecutor struct {
	registeredCarriers map[reflect.Type][]hooks.CalloutCarrier
}

// Constructs the hook executor using a list types of supported callout specifications.
func NewHookExecutor(calloutSpecificationTypes []reflect.Type) *HookExecutor {
	carriers := make(map[reflect.Type][]hooks.CalloutCarrier, len(calloutSpecificationTypes))
	for _, specificationType := range calloutSpecificationTypes {
		if specificationType.Kind() != reflect.Interface {
			// It should never happen.
			// If you got this panic message, check if your callout types are
			// defined as follows:
			// reflect.TypeOf((*hooks.FooCallout)(nil)).Elem()
			// remember about:
			// 1. pointer (star *) before the callout type.
			// 2. .Elem() call at the end.
			panic("non-interface type passed")
		}
		carriers[specificationType] = make([]hooks.CalloutCarrier, 0)
	}
	return &HookExecutor{
		registeredCarriers: carriers,
	}
}

// Registers a callout carrier in the hook executor. If it doesn't implement
// any supported specification then it's silently ignored.
func (he *HookExecutor) registerCalloutCarrier(carrier hooks.CalloutCarrier) {
	for specificationType, carriers := range he.registeredCarriers {
		if reflect.TypeOf(carrier).Implements(specificationType) {
			he.registeredCarriers[specificationType] = append(carriers, carrier)
		}
	}
}

// Unregisters all callout carriers by calling their Close methods.
func (he *HookExecutor) unregisterAllCalloutCarriers() []error {
	var errs []error

	for _, carriers := range he.registeredCarriers {
		for _, carrier := range carriers {
			err := carrier.Close()
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	he.registeredCarriers = make(map[reflect.Type][]hooks.CalloutCarrier)

	return errs
}

// Returns a slice of types of the supported callout specifications.
func (he *HookExecutor) GetTypesOfSupportedCalloutSpecifications() []reflect.Type {
	supportedTypes := make([]reflect.Type, len(he.registeredCarriers))
	i := 0
	for t := range he.registeredCarriers {
		supportedTypes[i] = t
		i++
	}
	return supportedTypes
}

// Returns true if a given callout specification is supported and has at least
// one callout carrier registered.
func (he *HookExecutor) HasRegistered(calloutSpecificationType reflect.Type) bool {
	carriers, ok := he.registeredCarriers[calloutSpecificationType]
	return ok && len(carriers) > 0
}

// Below are implemented helper functions to call the callouts. The proper
// approach to executing the hook code depends on a given hook's
// characteristics. Different cases will require different strategies. It's a
// short list of ideas on what helpers may be implemented.
//
// - Run all registered hooks sequentially
// - Run only one (first) registered hook
// - Run all registered hooks sequentially until the first failure
// - Run all registered hooks sequentially until the first success
// - Run all registered hooks sequentially until meeting the condition
// - Filter hooks by condition and run them sequentially
// - Run all (or conditionally selected) hooks parallel and wait for the finish
// - Run all (or conditionally selected) hooks parallel and forgot
// - Run all (or conditionally selected) hooks parallel and wait for the first finish

// Calls the specific callout using the caller object.
// It can be used to monitor performance in the future.
func callCallout[TSpecification any, TOutput any](carrier TSpecification, caller func(TSpecification) TOutput) TOutput {
	return caller(carrier)
}

// Calls the specific callout for all callout carriers sequentially, one by one.
func CallSequential[TSpecification any, TOutput any](he *HookExecutor, caller func(TSpecification) TOutput) []TOutput {
	t := reflect.TypeOf((*TSpecification)(nil)).Elem()
	carriers, ok := he.registeredCarriers[t]
	if !ok {
		return nil
	}

	var results []TOutput
	for _, carrier := range carriers {
		result := callCallout(carrier.(TSpecification), caller)
		results = append(results, result)
	}
	return results
}

// Calls the specific callout from a first callout carrier if any was
// registered. It is dedicated to cases when only one hook with a given callout
// is expected.
// Returns a default value if no callout was called.
func CallSingle[TSpecification any, TOutput any](he *HookExecutor, caller func(TSpecification) TOutput) (output TOutput) {
	t := reflect.TypeOf((*TSpecification)(nil)).Elem()
	carriers, ok := he.registeredCarriers[t]
	if !ok || len(carriers) == 0 {
		return
	} else if len(carriers) > 1 {
		logrus.
			WithField("carrier", t.Name()).
			Warn("there are many registered callout carriers but expected a single one")
	}
	return callCallout(carriers[0].(TSpecification), caller)
}

// Calls the callouts sequentially until first success. Success means non-empty
// data are returned, It is dedicated to implement the chain of responsibility
// pattern. Returns a default value if no callout was called.
func CallUntilSuccess[TSpecification any, TOutput comparable](he *HookExecutor, caller func(TSpecification) TOutput) (output TOutput) {
	t := reflect.TypeOf((*TSpecification)(nil)).Elem()
	var defaultOutput TOutput

	carriers, ok := he.registeredCarriers[t]
	if !ok {
		return
	}

	for _, carrier := range carriers {
		output = callCallout(carrier.(TSpecification), caller)
		if output != defaultOutput {
			return output
		}
	}
	return
}
