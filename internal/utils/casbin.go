package utils

import (
	"context"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"go.uber.org/zap"
)

const casbinModel = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act
`

// PolicyLoader is injected by the application.
// It should return Casbin policy CSV (loaded from DB, optionally cached in Redis).
// Set it in app.go before starting the server.
var PolicyLoader func(ctx context.Context) (string, error)

// SetPolicyLoader registers the CSV policy loader function.
func SetPolicyLoader(loader func(ctx context.Context) (string, error)) {
	PolicyLoader = loader
}

// CheckPermission returns true if any role in roles is granted access to path+method.
func CheckPermission(ctx context.Context, role string, path, method string) (bool, error) {
	if PolicyLoader == nil {
		Logger.Warn("casbin PolicyLoader not set — allowing all requests")
		return true, nil
	}

	csv, err := PolicyLoader(ctx)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(csv) == "" {
		return true, nil
	}

	e, err := newEnforcerFromCSV(csv)
	if err != nil {
		return false, err
	}
	ok, err := e.Enforce(role, path, method)
	if err != nil {
		Logger.Error("casbin enforce error", zap.String("role", role), zap.Error(err))
		return false, nil
	}
	if ok {
		return true, nil
	}

	return false, nil
}

// newEnforcerFromCSV builds a casbin enforcer from a CSV policy string.
// Each non-empty, non-comment line is expected to be: sub,obj,act
func newEnforcerFromCSV(csv string) (*casbin.Enforcer, error) {
	m, err := model.NewModelFromString(casbinModel)
	if err != nil {
		return nil, err
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(csv, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		e.AddPolicy(parts[0], parts[1], parts[2])
	}

	return e, nil
}
