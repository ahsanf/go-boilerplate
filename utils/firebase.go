package utils

import (
	"context"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

var FirebaseAuth *auth.Client

func InitFirebase(ctx context.Context) {
	credPath := Cfg.GoogleCredPath
	if credPath == "" {
		credPath = Cfg.ServiceAccount
	}

	var (
		app *firebase.App
		err error
	)

	if credPath != "" {
		app, err = firebase.NewApp(ctx, nil, option.WithCredentialsFile(credPath))
	} else {
		app, err = firebase.NewApp(ctx, nil)
	}
	if err != nil {
		Logger.Fatal("failed to init firebase app", zap.Error(err))
	}

	FirebaseAuth, err = app.Auth(ctx)
	if err != nil {
		Logger.Fatal("failed to init firebase auth", zap.Error(err))
	}

	Logger.Info("firebase initialized")
}
