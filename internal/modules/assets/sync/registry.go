package sync

import (
	"reflect"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
)

// Registry has no mutation methods. Composition supplies every provider once.
type Registry struct{ providers map[string]Provider }

func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil || nilProvider(provider) {
			return nil, planConfigError()
		}
		name := provider.Name()
		if name != "aws" && name != "aliyun" && name != "gcp" {
			return nil, planConfigError()
		}
		if _, exists := r.providers[name]; exists {
			return nil, planConfigError()
		}
		r.providers[name] = provider
	}
	return r, nil
}

func nilProvider(provider Provider) bool {
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (r *Registry) Lookup(name string) (Provider, error) {
	if r != nil {
		if provider, ok := r.providers[name]; ok {
			return provider, nil
		}
	}
	return nil, apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "cloud provider not supported")
}
