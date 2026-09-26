package goplatform

import (
	"reflect"
	"testing"
	"time"
)

func clearCommonEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "REDIS_URL", "LOG_LEVEL", "CORS_ALLOW_ORIGINS",
		"CORS_ALLOW_HEADERS", "SHUTDOWN_TIMEOUT", "DB_SLOW_QUERY_THRESHOLD", "DEPENDENCY_PING_TIMEOUT"} {
		t.Setenv(k, "")
	}
}

func TestLoadCommonRequiresDatabaseURL(t *testing.T) {
	clearCommonEnv(t)
	if _, err := LoadCommon(CommonDefaults{}); err == nil {
		t.Fatal("want error when DATABASE_URL is unset")
	}
}

func TestLoadCommonDefaults(t *testing.T) {
	clearCommonEnv(t)
	t.Setenv("DATABASE_URL", "postgres://test")

	c, err := LoadCommon(CommonDefaults{CORSAllowHeaders: []string{"Origin"}})
	if err != nil {
		t.Fatal(err)
	}
	want := CommonConfig{
		DatabaseURL:           "postgres://test",
		LogLevel:              "info",
		CORSAllowOrigins:      []string{"*"},
		CORSAllowHeaders:      []string{"Origin"},
		ShutdownTimeout:       10 * time.Second,
		DBSlowQueryThreshold:  2 * time.Second,
		DependencyPingTimeout: 3 * time.Second,
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v\nwant %+v", c, want)
	}
}

func TestLoadCommonRejectsBadValues(t *testing.T) {
	for key, val := range map[string]string{"LOG_LEVEL": "loud", "SHUTDOWN_TIMEOUT": "0s"} {
		clearCommonEnv(t)
		t.Setenv("DATABASE_URL", "postgres://test")
		t.Setenv(key, val)
		if _, err := LoadCommon(CommonDefaults{}); err == nil {
			t.Errorf("%s=%s: want error", key, val)
		}
	}
}
