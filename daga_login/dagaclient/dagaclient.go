///*
//* TODO FIXME
//* This is a template for creating an app.
// */
package main

//
//import (
//	"errors"
//	"fmt"
//	"github.com/BurntSushi/toml"
//	"github.com/dedis/kyber"
//	"github.com/dedis/kyber/util/encoding"
//	"github.com/dedis/onet/app"
//	"github.com/dedis/onet/log"
//	"github.com/dedis/onet/network"
//	"github.com/dedis/student_18_daga/daga_login"
//	"github.com/dedis/student_18_daga/sign/daga"
//	"gopkg.in/urfave/cli.v1"
//	"os"
//	"strconv"
//	"strings"
//)
//
//var suite = daga.NewSuiteEC()
//
//func main() {
//	cliApp := cli.NewApp()
//	cliApp.Usage = "Used for building other apps."
//	cliApp.Version = "0.1"
//	groupsDef := "the group-definition-file"
//	cliApp.Commands = []cli.Command{
//		{
//			Name:      "login",
//			Usage:     "create and send a new daga auth. request",
//			Aliases:   []string{"l"},
//			ArgsUsage: "the index (in auth. context) of the client being run",
//			Action:    cmdLogin,
//		},
//		{ // FIXME, for now here but move where more appropriate if still used later, used to provide a "boot method"
//			Name:      "setup",
//			Usage:     "setup c clients, servers and a daga auth. context and save them to FS TODO",
//			Aliases:   []string{"s"},
//			ArgsUsage: groupsDef + ", c the number of clients",
//			Action:    cmdSetup,
//		},
//	}
//	cliApp.Flags = []cli.Flag{
//		cli.IntFlag{
//			Name:  "debug, d",
//			Value: 0,
//			Usage: "debug-level: 1 for terse, 5 for maximal",
//		},
//	}
//	cliApp.Before = func(c *cli.Context) error {
//		log.SetDebugVisible(c.Int("debug"))
//		return nil
//	}
//	log.ErrFatal(cliApp.Run(os.Args))
//}
//
//// setup c clients, servers (from the configs found in ./mycononodes/cox and the group/roster) and a daga auth context and save them to FS
//// current hack way to provide daga context and identities to the participants
//// all the servers/nodes in the provided group/roster will be part of the generated Context
//// TODO FIXME out of scope for now (i'm hacking around to satisfy the assumptions of daga, but we will need protocols to setup a context, with start stop clientregister serverregister etc..))
//func cmdSetup(c *cli.Context) error {
//	// read group/roster
//	group := readGroup(c.Args())
//
//	// read number of clients
//	numClients := readInt(c.Args().Tail(), "Please give the number of DAGA clients you want to configure")
//	var errs []error
//
//	// read the servers keys from roster and private config files
//	serverKeys, err := readNodesPrivateKeys(group)
//	errs = append(errs, err)
//
//	network.RegisterMessages(daga_login.NetContext{}, daga_login.NetClient{}, daga_login.NetServer{})
//
//	// create daga clients servers and context and save them to FS
//	clients, servers, dagaContext, err := daga.GenerateContext(suite, numClients, serverKeys)
//	errs = append(errs, err)
//
//	//save context to new protobuf bin file TODO or whatever, maybe better toml config files
//	context, err := daga_login.NewContext(dagaContext, *group.Roster)
//	errs = append(errs, err)
//	netContext := context.NetEncode()
//	errs = append(errs, saveToFile("./context.bin", netContext)) // TODO remove magic strings
//
//	// save clients and servers conf to disk
//	netClients, err := daga_login.NetEncodeClients(clients)
//	errs = append(errs, err)
//	for i, netClient := range netClients {
//		errs = append(errs, saveToFile(fmt.Sprintf("./client%d.bin", i), &netClient)) // TODO remove magic strings
//	}
//	netServers, err := daga_login.NetEncodeServers(servers)
//	errs = append(errs, err)
//	for i, netServer := range netServers {
//		errs = append(errs, saveToFile(fmt.Sprintf("./server%d.bin", i), &netServer)) // TODO remove magic strings
//	}
//
//	for _, err := range errs {
//		if err != nil {
//			log.Fatal(err)
//		}
//	}
//	fmt.Println("done!")
//	return nil
//
//	// TODO proposals / ideas :
//	// 3rd party services that wants to delegate to a particular, up and running, DAGA cothority, their authentication mechanism need to: either:
//	// 1) build an authentication context from scratch using user provided keys (or provide users key pairs etc) while being
//	//		trusted to delete the private keys if they created them and be trusted to not keep any link between user and keys (or don't care depending on how the remaining part of system designed),
//	//		then ask nicely to the DAGA cothority to do its job providing it the context => register context API endpoint
//	//		=> poor man's DAGA... impossible to verify that service is nice => bad but why not
//	//		=> or depending on how the remaining parts are implemented can just work nicely
//	//
//	// 1) organize a PoP party to allow their potential user to register to the service (if need one person 1 user, else relaxed "Pop" party)
//	// 		build the auth context out of the party transcript
//	//		then ask nicely to the DAGA cothority to ... => register context API
//
//	// 2) register context: TODO etc..multiple choices for a conode admin, open up to N context or
//	//    DAGA slave cothority that use whatever context
//	//
//	// don't forget the context revocation end of round !! LOTS of implementations choices.... !
//}
//
//// authenticate
//func cmdLogin(c *cli.Context) error {
//	log.Info("Auth command")
//	index := readInt(c.Args(), "Please give the index (in auth. context) of the client you want to run")
//	// TODO same issues and questions as in setup => need to fix the frame/goals of my work, now I'm hacking to continue the developpment
//	// TODO all of this is not needed when there are facilities to create/join an auth. context, cmdLogin needs only a context, an index and a privatekey
//	// TODO but now I'll accept only index and parse private key from file(s) generated by setup (maybe I should move these meta things in a bash wrapper,  but..)
//	network.RegisterMessages(daga_login.NetClient{}, daga_login.NetContext{})
//	privateKey, err := daga_login.ReadClientPrivateKey(fmt.Sprintf("./client%d.bin", index))
//	if err != nil {
//		log.Fatal(err)
//	}
//	client, _ := daga_login.NewClient(index, privateKey)
//	context, err := daga_login.ReadContext("./context.bin") // TODO remove magic value
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	tag, err := client.Auth(context)
//	if err != nil {
//		return errors.New("Failed to login: " + err.Error())
//	}
//	log.Info("final linkage tag:", tag)
//	return nil
//}
//
//func readGroup(args cli.Args) *app.Group {
//	name := args.First()
//	if name == "" {
//		log.Fatal("Please give the group-file as argument")
//	}
//	f, err := os.Open(name)
//	log.ErrFatal(err, "Couldn't open group definition file")
//	group, err := app.ReadGroupDescToml(f)
//	log.ErrFatal(err, "Error while reading group definition file", err)
//	if len(group.Roster.List) == 0 {
//		log.ErrFatalf(err, "Empty entity or invalid group defintion in: %s",
//			name)
//	}
//	return group
//}
//
//func readInt(args cli.Args, errStr string) int {
//	intStr := args.First()
//	if intStr == "" {
//		log.Fatal(errStr)
//	}
//	if i, err := parseInt(intStr); err != nil {
//		log.Fatal(errStr + ": " + err.Error())
//		return -1
//	} else {
//		return i
//	}
//}
//
//func parseInt(intStr string) (int, error) {
//	if index, err := strconv.Atoi(intStr); err == nil {
//		return index, nil
//	} else {
//		return -1, err
//	}
//}
//
//// msg must be a pointer to data type registered to the network library
//func saveToFile(path string, msg interface{}) error {
//	bytes, err := network.Marshal(msg)
//	if err != nil {
//		return errors.New("saveToFile: " + err.Error())
//	}
//	fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
//	if err != nil {
//		return errors.New("saveToFile: " + err.Error())
//	}
//	_, err = fd.Write(bytes)
//	if err != nil {
//		return errors.New("saveToFile: " + err.Error())
//	}
//	return nil
//}
//
//func readNodesPrivateKeys(group *app.Group) ([]kyber.Scalar, error) {
//	// TODO here assume that the conode description are always something_N in roster and N maps to /path/to/conode/mycononodes/coN
//	// (it is the case when using run_conodes.sh) maybe add possibility to specify basepath and delimiter or map[description]index
//	// FIXME QUESTION dumb, instead here generate the private.toml files from scratch using infos from daga.GenerateContext (but guess I'll lose compatibility with other facilities, bash tests etc..)
//	// FIXME + more sound since now I don't check the suite in toml but... if want same functionnality as the one given by run_conodes.sh will took time...
//	// FIXME or quickfix check suite ..
//	// TODO anyway this is kind of temp hack for now, should be taken care by login service (context establishement register
//	serverKeys := make([]kyber.Scalar, 0, len(group.Description))
//	for _, description := range group.Description {
//		iStr := description[strings.IndexByte(description, '_')+1:]
//		if i, err := strconv.Atoi(iStr); err != nil {
//			return nil, errors.New("failed to get server index out of \"" + description + "\": " + err.Error())
//		} else {
//			// TODO remove magic strings
//			path := "/home/lpi/msthesis/work/dedis_go_srcs/student_18_daga/daga_login/conode/myconodes/co" + iStr + "/private.toml"
//			config := &app.CothorityConfig{}
//			_, err := toml.DecodeFile(path, config)
//			if err != nil {
//				return nil, fmt.Errorf("failed to parse the cothority config of server %d: %s", i, err)
//			}
//			privateKey, err := encoding.StringHexToScalar(suite, config.Private) // FIXME let's hope that this will work (the suite..)
//			serverKeys = append(serverKeys, privateKey)
//		}
//	}
//	return serverKeys, nil
//}
