package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/datastore"
	"github.com/GannettDigital/go-vault-utility"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// RBACTeamAssets - struct to hold data on what records and healthchecks a team has permission to edit
type RBACTeamAssets struct {
	TeamName     string   `json:"teamname" validate:"nonzero"`
	DNSRecords   []string `json:"dnsRecords"`
	HealthChecks []string `json:"healthchecks"`
}

var (
	err   error
	retT1 RBACTeamAssets
	retT2 RBACTeamAssets
)

const ()

func populateSecret(vaultController *vaultutil.Controller, path string, secret string) string {
	s, err := vaultController.GetSecretFieldString(path, secret)
	if err != nil {
		log.Fatalf("error: unable to load secret %s/%s. %v", path, secret, err)
	}
	return s
}

func main() {

	vaultBasePath := "secret/sre/datastore/carl-demo"
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	vaultController, err := vaultutil.NewController(vaultutil.Config{
		Logger: logger,
		// VaultRole: "dns-manager",
	})
	if err != nil {
		logger.Fatalf("error: failed to initialize new vault controller: %v", err)
	}
	logger.Printf("successfully initialized vault controller instance")
	// credjson := populateSecret(vaultController, vaultBasePath, "config")
	// os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", fmt.Sprintf(populateSecret(vaultController, vaultBasePath, "config")))
	// fmt.Println(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	t1 := RBACTeamAssets{
		TeamName:     "marsVolta",
		DNSRecords:   []string{"deloused.in.the.comatorium", "bedlam.in.goliath"},
		HealthChecks: []string{"eriatarka", "wax.simulacra"},
	}

	t2 := RBACTeamAssets{
		TeamName:     "karnivool",
		DNSRecords:   []string{"sound.awake", "themata"},
		HealthChecks: []string{"simple.boy", "shutterspeed"},
	}

	// client
	ctx := context.Background()

	data := fmt.Sprintf(populateSecret(vaultController, vaultBasePath, "config"))
	if err != nil {
		log.Fatal(err)
	}

	creds, err := google.CredentialsFromJSON(ctx, []byte(data), datastore.ScopeDatastore)
	if err != nil {
		log.Fatal(err)
	}

	client, err := datastore.NewClient(ctx, "ds-example-250420", option.WithCredentials(creds))
	if err != nil {
		log.Fatal(err)
		return
	}

	// populate
	errCheck(populateDatastore(ctx, t1, client))
	errCheck(populateDatastore(ctx, t2, client))

	// get assets
	// this would be implemented in GetTeamAssets
	retT1, err = getAsset(ctx, "karnivool", client)
	errCheck(err)
	retT2, err = getAsset(ctx, "marsVolta", client)
	errCheck(err)
	fmt.Println("all assets: ", retT1, retT2)

	// get all name keys
	// This would be implemented in GetAllOwners
	keys, err := getAllKeys(ctx, client)
	errCheck(err)
	fmt.Println("all owners: ", keys)

	// get specific key owner
	// This would be implemented in GetRecordOwner
	teamName, err := getRecordTeam(ctx, "sound.awake", client)
	errCheck(err)
	fmt.Println(teamName)
	teamName, err = getRecordTeam(ctx, "themata", client)
	errCheck(err)
	fmt.Println(teamName)
	teamName, err = getRecordTeam(ctx, "nope", client)
	// errCheck(err) passing err check since we know this will fail
	fmt.Println(err)

	// update team asset
	t2Prime := t2
	t2Prime.DNSRecords = append(t2Prime.DNSRecords, "t2r3")
	err = updateTeamAssets(ctx, "karnivool", t2Prime, client)
	errCheck(err)
	// get record after update
	retT2, err = getAsset(ctx, "karnivool", client)
	errCheck(err)
	fmt.Println("updated karnivool assets: ", retT2)
}

func populateDatastore(ctx context.Context, ta RBACTeamAssets, client *datastore.Client) error {
	teamAssetKey := datastore.NameKey("sgune", ta.TeamName, nil)
	_, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var empty RBACTeamAssets
		if err := tx.Get(teamAssetKey, &empty); err != datastore.ErrNoSuchEntity {
			return err
		}
		_, err := tx.Put(teamAssetKey, &ta)
		return err
	})
	return err
}

func getAsset(ctx context.Context, teamName string, client *datastore.Client) (RBACTeamAssets, error) {
	var teamAsset RBACTeamAssets
	teamAssetKey := datastore.NameKey("sgune", teamName, nil)
	err := client.Get(ctx, teamAssetKey, &teamAsset)
	return teamAsset, err
}

func getRecordTeam(ctx context.Context, record string, client *datastore.Client) (string, error) {
	var (
		err error
		tas []RBACTeamAssets
	)
	query := datastore.NewQuery("sgune").Filter("DNSRecords=", record)
	it := client.Run(ctx, query)
	for {
		var ta RBACTeamAssets
		_, err := it.Next(&ta)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Error fetching next task: %v", err)
		}
		tas = append(tas, ta)
	}
	if len(tas) > 1 {
		return "", fmt.Errorf("dns record is NOT unique among teams")
	}
	if len(tas) == 0 {
		return "", fmt.Errorf("no dns record found for %s", record)
	}

	return tas[0].TeamName, err
}

func getAllKeys(ctx context.Context, client *datastore.Client) ([]string, error) {
	var keyStrings []string
	query := datastore.NewQuery("sgune").KeysOnly()
	keys, err := client.GetAll(ctx, query, nil)
	for _, key := range keys {
		keyStrings = append(keyStrings, key.Name)
	}
	return keyStrings, err
}

func updateTeamAssets(ctx context.Context, teamName string, asset RBACTeamAssets, client *datastore.Client) error {
	var tmp RBACTeamAssets
	teamAssetKey := datastore.NameKey("sgune", asset.TeamName, nil)
	tx, err := client.NewTransaction(ctx)
	if err := tx.Get(teamAssetKey, &tmp); err != nil {
		// team dont exist
		return fmt.Errorf("tx.Get: %v", err)
	}
	// could do some cross checking with tmp here but since we know what is in team2 asset, lets just push up "asset"
	if _, err := tx.Put(teamAssetKey, &asset); err != nil {
		return fmt.Errorf("tx.Put: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		return fmt.Errorf("tx.Commit: %v", err)
	}
	// wont get here but w/e
	return err
}

func errCheck(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

