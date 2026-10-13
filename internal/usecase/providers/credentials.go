package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

const KeptCredential = "__PI_SWITCH_KEEP_CREDENTIAL__"

var ErrRetainedCredential = errors.New("retained credential has no matching source; reload or enter a replacement")

// MaskCredentials returns a detached draft and keeps the original values only in
// the caller's in-memory retention map.
func MaskCredentials(profile provider.Profile) (provider.Profile, map[string]string, error) {
	data, err := json.Marshal(profile)
	if err != nil {
		return provider.Profile{}, nil, err
	}
	var masked provider.Profile
	if err := json.Unmarshal(data, &masked); err != nil {
		return masked, nil, err
	}
	retained := make(map[string]string)
	mask := func(value *string, field string) {
		if *value == "" {
			return
		}
		reference := KeptCredential + field
		retained[reference], *value = *value, reference
	}
	credentials := func(endpoint, key *string, headers map[string]string, scope string) {
		mask(endpoint, scope+".baseUrl")
		mask(key, scope+".apiKey")
		for name, value := range headers {
			mask(&value, scope+".headers."+name)
			headers[name] = value
		}
	}
	credentials(&masked.BaseURL, &masked.APIKey, masked.Headers, "profile")
	for i := range masked.Upstreams {
		u := &masked.Upstreams[i]
		credentials(&u.BaseURL, &u.APIKey, u.Headers, fmt.Sprintf("upstreams[%d]", i))
	}
	return masked, retained, nil
}

func RestoreCredentials(profile *provider.Profile, retained map[string]string) error {
	restore := func(value *string) error {
		if !strings.HasPrefix(*value, KeptCredential) {
			return nil
		}
		original, ok := retained[*value]
		if !ok {
			return ErrRetainedCredential
		}
		*value = original
		return nil
	}
	credentials := func(endpoint, key *string, headers map[string]string) error {
		if err := restore(endpoint); err != nil {
			return err
		}
		if err := restore(key); err != nil {
			return err
		}
		for name, value := range headers {
			if err := restore(&value); err != nil {
				return err
			}
			headers[name] = value
		}
		return nil
	}
	if err := credentials(&profile.BaseURL, &profile.APIKey, profile.Headers); err != nil {
		return err
	}
	for i := range profile.Upstreams {
		u := &profile.Upstreams[i]
		if err := credentials(&u.BaseURL, &u.APIKey, u.Headers); err != nil {
			return err
		}
	}
	return nil
}
