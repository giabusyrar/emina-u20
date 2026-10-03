package inference_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
	"github.com/experience-gateway/apps/core-engine/internal/vision/inference"
)

func TestHTTPCapabilityClient_SendsCropsAndDecodesScores(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/models/dispatch-zones", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body struct {
			Capabilities []string `json:"capabilities"`
			Zones        []struct {
				ZoneCode    string `json:"zoneCode"`
				SourceAngle string `json:"sourceAngle"`
				Image       string `json:"image"`
			} `json:"zones"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"acne_lesion_classifier", "hyperpigmentation_net"}, body.Capabilities)
		if assert.Len(t, body.Zones, 1) {
			assert.Equal(t, "ZONE_LEFT_CHEEK_OUTER", body.Zones[0].ZoneCode)
			assert.Equal(t, "LEFT", body.Zones[0].SourceAngle)
			decoded, err := base64.StdEncoding.DecodeString(body.Zones[0].Image)
			assert.NoError(t, err)
			assert.Equal(t, []byte("crop-1"), decoded)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"zoneCode":"ZONE_LEFT_CHEEK_OUTER","sourceAngle":"LEFT","telemetry":{"acne_lesion_classifier":42.5}}],"unsupportedCapabilities":["hyperpigmentation_net"]}`))
	}))
	defer server.Close()

	client := inference.NewHTTPCapabilityClient(server.URL+"/", 5*time.Second)
	result, err := client.DispatchZones(context.Background(),
		[]string{"acne_lesion_classifier", "hyperpigmentation_net"},
		[]inference.ZoneCrop{{ZoneCode: domain.ZoneLeftCheekOuter, SourceAngle: domain.AngleLeft, Image: []byte("crop-1")}},
	)

	require.NoError(t, err)
	require.Len(t, result.Zones, 1)
	assert.Equal(t, domain.ZoneLeftCheekOuter, result.Zones[0].ZoneCode)
	assert.Equal(t, domain.AngleLeft, result.Zones[0].SourceAngle)
	assert.Equal(t, 42.5, result.Zones[0].Telemetry["acne_lesion_classifier"])
	assert.Equal(t, []string{"hyperpigmentation_net"}, result.UnsupportedCapabilities)
}

func TestHTTPCapabilityClient_NonSuccessStatusIsWorkerUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := inference.NewHTTPCapabilityClient(server.URL, 5*time.Second).DispatchZones(context.Background(), []string{"a"}, []inference.ZoneCrop{{ZoneCode: "Z"}})

	assert.True(t, errors.Is(err, inference.ErrWorkerUnavailable))
}

func TestHTTPCapabilityClient_MalformedResponseIsWorkerUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := inference.NewHTTPCapabilityClient(server.URL, 5*time.Second).DispatchZones(context.Background(), []string{"a"}, []inference.ZoneCrop{{ZoneCode: "Z"}})

	assert.True(t, errors.Is(err, inference.ErrWorkerUnavailable))
}

func TestHTTPCapabilityClient_UnreachableIsWorkerUnavailable(t *testing.T) {
	_, err := inference.NewHTTPCapabilityClient("http://127.0.0.1:1", 5*time.Second).DispatchZones(context.Background(), []string{"a"}, []inference.ZoneCrop{{ZoneCode: "Z"}})

	assert.True(t, errors.Is(err, inference.ErrWorkerUnavailable))
}

func TestHTTPCapabilityClient_ContextDeadlineIsWorkerUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := inference.NewHTTPCapabilityClient(server.URL, 5*time.Second).DispatchZones(ctx, []string{"a"}, []inference.ZoneCrop{{ZoneCode: "Z"}})

	assert.True(t, errors.Is(err, inference.ErrWorkerUnavailable))
}

func TestHTTPCapabilityClient_DecodesUnavailableCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"unsupportedCapabilities":["acne_lesion_classifier"],"unavailableCapabilities":{"acne_lesion_classifier":"RuntimeError: S3 unreachable"}}`))
	}))
	defer server.Close()

	result, err := inference.NewHTTPCapabilityClient(server.URL, 5*time.Second).DispatchZones(context.Background(), []string{"acne_lesion_classifier"}, []inference.ZoneCrop{{ZoneCode: "Z"}})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"acne_lesion_classifier": "RuntimeError: S3 unreachable"}, result.UnavailableCapabilities)
}
