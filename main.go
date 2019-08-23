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

// import (
// 	"encoding/json"
// 	"fmt"
// 	"io/ioutil"
// 	"os"

// 	"github.com/cloudflare/cfssl/log"
// 	"github.com/hashicorp/consul/api"
// 	consulAPI "github.com/hashicorp/consul/api"
// 	"github.com/nlopes/slack"
// )

// // interactiveHandler holds information needed for handling interactive messages.
// type interactiveHandler struct {
// 	// client is our connection to the slack api.
// 	client *slack.Client

// 	// payload is what was recieved from slack to our interactive endpoint.
// 	payload *slack.DialogCallback

// 	// url is the response url
// 	url string

// 	// consulClient used to manage subscriptions
// 	consulClient *consulAPI.Client
// }

// type statusPage struct {
// 	ID          string `json:"members.id"`
// 	Name        string
// 	Group_id    string
// 	Description string
// }

// //Users Structs to parse slack user info
// type Users struct {
// 	Users []User `json:"members"`
// }

// //User is a list of variables for a slack user
// type User struct {
// 	ID       string `json:"id"`
// 	Name     string `json:"name"`
// 	RealName string `json:"real_name"`
// }

// // KVPair is used to represent a single K/V entry
// type KVPair struct {
// 	Key         string
// 	CreateIndex uint64
// 	ModifyIndex uint64
// 	LockIndex   uint64
// 	Flags       uint64
// 	Value       []byte
// 	Session     string
// }

// // KVPairs is a list of KVPair objects
// type KVPairs []*KVPair

// var statusID = "yppf44cd6dkl"
// var channelID = "CCK59RFN0"
// var apiBaseUrl = "https://slack.com/api/"
// var endpoint = "conversations.invite"

// func addUsers() {
// 	filePath := "./slackusers.json"
// 	fmt.Printf("// reading file %s\n", filePath)
// 	file, err1 := ioutil.ReadFile(filePath)
// 	if err1 != nil {
// 		fmt.Printf("// error while reading file %s\n", filePath)
// 		fmt.Printf("File error: %v\n", err1)
// 		os.Exit(1)
// 	}

// 	fmt.Println("// defining array of struct Slackers")
// 	var slackers Users

// 	err2 := json.Unmarshal(file, &slackers)
// 	if err2 != nil {
// 		fmt.Println("error:", err2)
// 		os.Exit(1)
// 	}

// 	// fmt.Println("// loop over array of users for slack")
// 	// for u := range slackers.Users {
// 	// 	//fmt.Printf("The component '%s' is: '%s'\n", slackers.Users[u].ID, slackers.Users[u].Name)
// 	// 	// fmt.Printf("Real Name: %s\n", slackers.Users[u].RealName)
// 	// }
// 	// Get a new client
// 	client, err := api.NewClient(api.DefaultConfig())
// 	if err != nil {
// 		panic(err)
// 	}

// 	// Get a handle to the KV API
// 	kv := client.KV()

// 	// PUT a new KV pair
// 	for u := range slackers.Users {
// 		// fmt.Printf("Inserting component '%s' is: '%s'\n", slackers.Users[u].ID, slackers.Users[u].Name)
// 		u := &api.KVPair{Key: ("Slackers/" + slackers.Users[u].Name), Value: []byte(slackers.Users[u].ID)}

// 		_, err = kv.Put(u, nil)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}
// }

// func subscribeUsers() {
// 	// Get a new client
// 	client, err := api.NewClient(api.DefaultConfig())
// 	if err != nil {
// 		panic(err)
// 	}

// 	// Get a handle to the KV API
// 	kv := client.KV()

// 	// PUT a new KV pair
// 	s := &api.KVPair{Key: "subscribers/yppf44cd6dkl/sgune", Value: []byte("U8S600XNG")}
// 	_, err = kv.Put(s, nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	// PUT a new KV pair
// 	c := &api.KVPair{Key: "subscribers/yppf44cd6dkl/ceichhorn", Value: []byte("U8UFB5LS0")}
// 	_, err = kv.Put(c, nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	// PUT a new KV pair
// 	a := &api.KVPair{Key: "subscribers/yppf44cd6dkl/aullah", Value: []byte("UB210A483")}
// 	_, err = kv.Put(a, nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	// PUT a new KV pair
// 	r := &api.KVPair{Key: "subscribers/yppf44cd6dkl/rgindes", Value: []byte("U8SA996RK")}
// 	_, err = kv.Put(r, nil)
// 	if err != nil {
// 		panic(err)
// 	}

// 	//  Look up subscribers

// }

// //addToChannel will invite a user to our authenticated Slack
// func addToChannel() {

// 	// Get a new client

// 	fmt.Println("Setting up the client ")
// 	client, err := api.NewClient(api.DefaultConfig())
// 	if err != nil {
// 		panic(err)
// 	}
// 	kv := client.KV()

// 	//Get a list of keys for a path

// 	fmt.Println("  Getting a list of keys ---")

// 	lst, _, err := kv.Keys(fmt.Sprintf("subscribers/%s/", statusID), "/", nil)
// 	if err != nil {
// 		log.Error(err)
// 	}
// 	//log.Info(lst)
// 	fmt.Println(lst)
// 	fmt.Println(" getting just one entry using dereference ---")
// 	//fmt.Println((lst)[0])
// 	// Lookup the pair and return 1

// 	for i, v := range lst {

// 		fmt.Printf("Index: %d and Val: %v\n", i, v)
// 		val, _, err := kv.Get(v, nil)

// 		fmt.Printf("Just value: %s\n", val.Value)

// 		if err != nil {
// 			log.Error(err)

// 		}
// 	}
// 	//  printing just 1 hard coded
// 	fmt.Println("---------   Printing just 1  -----")
// 	sin, _, err := kv.Get((lst)[1], nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	fmt.Printf("Inviting you to a Channel: %v %s\n", sin.Key, sin.Value)
// 	fmt.Println("Just value: ", string(sin.Value))
// 	api := slack.New(os.Getenv("SLACK_TOKEN"))

// 	//  Here is were we create the slack invite message
// 	params := slack.PostMessageParameters{Attachments: []slack.Attachment{
// 		{
// 			Pretext: "Component:  Testing it.   With severity:  Not severe, in fact it's fine.",
// 			Text:    ("To Join the Channel click <#" + channelID + "|Channel>"),
// 		},
// 	}}
// 	_, timestamp, err := api.PostMessage(string(sin.Value), ("A Slack Channel has been created to troubleshoot an issue with " + statusID), params)
// 	if err != nil {
// 		log.Info(fmt.Sprintf("Message failed to send to user %s at %s", sin.Value, timestamp))
// 		log.Error(err)
// 	}

// }
// func main() {
// 	addUsers()
// 	subscribeUsers()
// 	addToChannel()
// }
