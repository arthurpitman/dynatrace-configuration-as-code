package stubs

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Stub struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	EntityID string  `json:"entityId,omitempty"`
	Value    any     `json:"-"`
	LegacyID *string `json:"legacyID,omitempty"`
	ParentID *string `json:"parentID,omitempty"`
}

var mu sync.Mutex
var allStubs map[string][]Stub = map[string][]Stub{}

func getStubsFolder() string {
	return strings.TrimSpace(os.Getenv("DYNATRACE_STUBS_FOLDER"))
}

func shouldWriteStubs() bool {
	return getStubsFolder() != ""
}

func RecordStub(s Stub, t string) {
	mu.Lock()
	defer mu.Unlock()
	allStubs[t] = append(allStubs[t], s)
}

func WriteAllStubs() error {
	mu.Lock()
	defer mu.Unlock()

	if !shouldWriteStubs() {
		return nil
	}

	for name, v := range allStubs {
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}

		filename := fmt.Sprintf("%s/%s.json", getStubsFolder(), name)
		err = os.WriteFile(filename, bytes, 0644)
		if err != nil {
			return fmt.Errorf("failed to write stubs file: %w", err)
		}
	}

	return nil
}

func WriteStubsValue(name string, id string, v string) error {
	if !shouldWriteStubs() {
		return nil
	}

	filename := fmt.Sprintf("%s/%s_%s.json", getStubsFolder(), name, id)
	err := os.WriteFile(filename, []byte(v), 0644)
	if err != nil {
		return fmt.Errorf("failed to write stubs value file: %w", err)
	}

	return nil
}
