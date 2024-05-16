package resources

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gotidy/ptr"

	firebase "firebase.google.com/go"
	liberror "github.com/ekristen/libnuke/pkg/errors"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/settings"
	"github.com/ekristen/libnuke/pkg/types"

	"github.com/ekristen/gcp-nuke/pkg/gcputil"
	"github.com/ekristen/gcp-nuke/pkg/nuke"
)

const FirebaseRealtimeDatabaseResource = "FirebaseRealtimeDatabase"

func init() {
	registry.Register(&registry.Registration{
		Name:   FirebaseRealtimeDatabaseResource,
		Scope:  nuke.Project,
		Lister: &FirebaseRealtimeDatabaseLister{},
		Settings: []string{
			"EmptyDefaultDatabase",
		},
	})
}

type FirebaseRealtimeDatabaseLister struct {
	svc *gcputil.FirebaseDBClient
}

func (l *FirebaseRealtimeDatabaseLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	if *opts.Region == "global" {
		return nil, liberror.ErrSkipRequest("resource is regional")
	}

	var resources []resource.Resource

	if l.svc == nil {
		var err error
		l.svc, err = gcputil.NewFirebaseDBClient(ctx)
		if err != nil {
			return nil, err
		}
	}

	supportedRegions := l.svc.ListDatabaseRegions()
	if !slices.Contains(supportedRegions, *opts.Region) {
		return nil, liberror.ErrSkipRequest("region is not supported")
	}

	resp, err := l.svc.ListDatabaseInstances(ctx, fmt.Sprintf("projects/%s/locations/%s", *opts.Project, *opts.Region))
	if err != nil {
		return nil, err
	}

	for _, instance := range resp {
		nameParts := strings.Split(instance.Name, "/")
		name := nameParts[len(nameParts)-1]

		if instance.Type == "DEFAULT_DATABASE" && instance.State == "DISABLED" {
			continue
		}

		resources = append(resources, &FirebaseRealtimeDatabase{
			svc:      l.svc,
			Project:  opts.Project,
			Region:   opts.Region,
			Name:     ptr.String(name),
			FullName: ptr.String(instance.Name),
			Type:     ptr.String(instance.Type),
			State:    ptr.String(instance.State),
			URL:      ptr.String(instance.DatabaseURL),
		})

	}

	return resources, nil
}

type FirebaseRealtimeDatabase struct {
	svc      *gcputil.FirebaseDBClient
	settings *settings.Setting
	Project  *string
	Region   *string
	Name     *string
	FullName *string `property:"-"`
	Type     *string
	State    *string
	URL      *string
}

func (r *FirebaseRealtimeDatabase) Remove(ctx context.Context) error {
	if err := r.EmptyDefaultDatabase(ctx); err != nil {
		return err
	}

	if err := r.DisableDatabaseInstance(ctx); err != nil {
		return err
	}

	return r.DeleteDatabaseInstance(ctx)
}

func (r *FirebaseRealtimeDatabase) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *FirebaseRealtimeDatabase) Settings(settings *settings.Setting) {
	r.settings = settings
}

func (r *FirebaseRealtimeDatabase) String() string {
	return *r.Name
}

func (r *FirebaseRealtimeDatabase) DeleteDatabaseInstance(ctx context.Context) error {
	// If it is the default database, it cannot be deleted only disabled.
	if *r.Type == "DEFAULT_DATABASE" {
		return nil
	}

	return r.svc.DeleteDatabaseInstance(ctx,
		fmt.Sprintf("projects/%s/locations/%s", *r.Project, *r.Region), *r.Name)
}

func (r *FirebaseRealtimeDatabase) EmptyDefaultDatabase(ctx context.Context) error {
	if r.settings == nil {
		return nil
	}

	// If it is not the default database then we just skip
	if *r.Type != "DEFAULT_DATABASE" {
		return nil
	}

	// If the setting is not enabled, then we just skip
	if !r.settings.Get("EmptyDefaultDatabase").(bool) {
		return nil
	}

	firebaseApp, err := firebase.NewApp(ctx, &firebase.Config{
		DatabaseURL: *r.URL,
	})
	if err != nil {
		return err
	}

	firebaseDb, err := firebaseApp.Database(ctx)
	if err != nil {
		return err
	}

	return firebaseDb.NewRef("/").Delete(ctx)
}

func (r *FirebaseRealtimeDatabase) DisableDatabaseInstance(ctx context.Context) error {
	if err := r.svc.DisableDatabaseInstance(ctx,
		fmt.Sprintf("projects/%s/locations/%s", *r.Project, *r.Region), *r.Name); err != nil {
		return err
	}

	return nil
}
