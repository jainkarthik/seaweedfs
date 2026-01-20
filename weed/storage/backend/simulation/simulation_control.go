package simulation

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/glog"
)

type SimulationControl struct {
	mu              sync.RWMutex
	storageFiles    map[string]*SimulatedBackendStorageFile
	globalConfig    SimulationConfig
	enabled         bool
	metricsHandlers map[string]*SimulationMetrics
}

var globalControl = &SimulationControl{
	storageFiles:    make(map[string]*SimulatedBackendStorageFile),
	globalConfig:    DefaultSimulationConfig(),
	enabled:         false,
	metricsHandlers: make(map[string]*SimulationMetrics),
}

func DefaultSimulationConfig() SimulationConfig {
	return SimulationConfig{
		Latency:       5 * time.Millisecond,
		ErrorRate:     0.0,
		Bandwidth:     0,
		DiskSpace:     0,
		EnableFailure: false,
		FailurePoint:  0,
	}
}

func RegisterStorageFile(name string, file *SimulatedBackendStorageFile) {
	globalControl.mu.Lock()
	defer globalControl.mu.Unlock()

	globalControl.storageFiles[name] = file
	metrics := file.GetMetrics()
	globalControl.metricsHandlers[name] = &metrics
}

func GetGlobalControl() *SimulationControl {
	return globalControl
}

// HTTP Handlers

func (sc *SimulationControl) HandleStatus(w http.ResponseWriter, r *http.Request) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	status := map[string]interface{}{
		"enabled":             sc.enabled,
		"global_config":       sc.globalConfig,
		"storage_files":       len(sc.storageFiles),
		"total_bytes_written": sc.getTotalBytesWritten(),
		"total_bytes_read":    sc.getTotalBytesRead(),
		"total_operations":    sc.getTotalOperations(),
		"total_errors":        sc.getTotalErrors(),
		"average_latency":     sc.getAverageLatency(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (sc *SimulationControl) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	fileName := r.URL.Query().Get("file")

	if fileName != "" {
		if file, exists := sc.storageFiles[fileName]; exists {
			metrics := file.GetMetrics()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(metrics)
		} else {
			http.Error(w, "Storage file not found", http.StatusNotFound)
		}
	} else {
		// Return all metrics
		allMetrics := make(map[string]*SimulationMetrics)
		for name, file := range sc.storageFiles {
			metrics := file.GetMetrics()
			allMetrics[name] = &metrics
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(allMetrics)
	}
}

func (sc *SimulationControl) HandleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sc.handleGetConfig(w, r)
	case http.MethodPost:
		sc.handleSetConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (sc *SimulationControl) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	fileName := r.URL.Query().Get("file")

	if fileName != "" {
		if file, exists := sc.storageFiles[fileName]; exists {
			config := file.GetConfig()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(config)
		} else {
			http.Error(w, "Storage file not found", http.StatusNotFound)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sc.globalConfig)
	}
}

func (sc *SimulationControl) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var newConfig SimulationConfig
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	fileName := r.URL.Query().Get("file")

	if fileName != "" {
		if file, exists := sc.storageFiles[fileName]; exists {
			file.UpdateConfig(newConfig)
			glog.V(0).Infof("Updated simulation config for file: %s", fileName)
		} else {
			http.Error(w, "Storage file not found", http.StatusNotFound)
			return
		}
	} else {
		sc.globalConfig = newConfig
		// Update all files with new global config
		for _, file := range sc.storageFiles {
			file.UpdateConfig(newConfig)
		}
		glog.V(0).Infof("Updated global simulation config")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (sc *SimulationControl) HandleReset(w http.ResponseWriter, r *http.Request) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	fileName := r.URL.Query().Get("file")

	if fileName != "" {
		if file, exists := sc.storageFiles[fileName]; exists {
			file.ResetMetrics()
			glog.V(0).Infof("Reset metrics for file: %s", fileName)
		} else {
			http.Error(w, "Storage file not found", http.StatusNotFound)
			return
		}
	} else {
		// Reset all files
		for _, file := range sc.storageFiles {
			file.ResetMetrics()
		}
		glog.V(0).Infof("Reset all simulation metrics")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

func (sc *SimulationControl) HandleEnable(w http.ResponseWriter, r *http.Request) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.enabled = true
	glog.V(0).Infof("Simulation backend enabled")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": sc.enabled,
		"status":  "enabled",
	})
}

func (sc *SimulationControl) HandleDisable(w http.ResponseWriter, r *http.Request) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.enabled = false
	glog.V(0).Infof("Simulation backend disabled")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": sc.enabled,
		"status":  "disabled",
	})
}

// Helper methods for metrics aggregation

func (sc *SimulationControl) getTotalBytesWritten() int64 {
	var total int64
	for _, file := range sc.storageFiles {
		total += file.GetMetrics().BytesWritten
	}
	return total
}

func (sc *SimulationControl) getTotalBytesRead() int64 {
	var total int64
	for _, file := range sc.storageFiles {
		total += file.GetMetrics().BytesRead
	}
	return total
}

func (sc *SimulationControl) getTotalOperations() int64 {
	var total int64
	for _, file := range sc.storageFiles {
		metrics := file.GetMetrics()
		total += metrics.WriteOperations + metrics.ReadOperations
	}
	return total
}

func (sc *SimulationControl) getTotalErrors() int64 {
	var total int64
	for _, file := range sc.storageFiles {
		total += file.GetMetrics().ErrorsInjected
	}
	return total
}

func (sc *SimulationControl) getAverageLatency() time.Duration {
	if len(sc.storageFiles) == 0 {
		return 0
	}

	var totalLatency time.Duration
	count := 0

	for _, file := range sc.storageFiles {
		latency := file.GetMetrics().AverageLatency
		if latency > 0 {
			totalLatency += latency
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return totalLatency / time.Duration(count)
}

// Setup HTTP handlers
func SetupHTTPHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/simulation/status", globalControl.HandleStatus)
	mux.HandleFunc("/simulation/metrics", globalControl.HandleMetrics)
	mux.HandleFunc("/simulation/config", globalControl.HandleConfig)
	mux.HandleFunc("/simulation/reset", globalControl.HandleReset)
	mux.HandleFunc("/simulation/enable", globalControl.HandleEnable)
	mux.HandleFunc("/simulation/disable", globalControl.HandleDisable)

	glog.V(0).Infof("Simulation backend HTTP handlers registered at /simulation/*")
}

// Utility methods for integration with existing systems

func (sc *SimulationControl) SetGlobalConfig(config SimulationConfig) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.globalConfig = config
	for _, file := range sc.storageFiles {
		file.UpdateConfig(config)
	}
}

func (sc *SimulationControl) IsEnabled() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.enabled
}

func (s *SimulatedBackendStorageFile) GetConfig() SimulationConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}
