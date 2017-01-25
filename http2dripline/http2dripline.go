package main

import (
    //"bytes"
    "flag"
    "fmt"
    "net/http"
    "net/http/httputil"
    "os"
    "os/user"
    //"strings"
    "encoding/json"
    "time"

    "github.com/kardianos/osext"
    "github.com/spf13/viper"

    "github.com/project8/dripline-go/dripline"
    "github.com/project8/swarm/Go/authentication"
    "github.com/project8/swarm/Go/logging"
)

var MasterSenderInfo dripline.SenderInfo
func fillMasterSenderInfo() (e error) {
    MasterSenderInfo.Package = "http2dripline"
    MasterSenderInfo.Exe, e = osext.Executable()
    if e != nil {
        return
    }

    //MasterSenderInfo.Version = gogitver.Tag()
    //MasterSenderInfo.Commit = gogitver.Git()

    MasterSenderInfo.Hostname, e = os.Hostname()
    if e != nil {
        return
    }

    user, userErr := user.Current()
    e = userErr
    if e != nil {
        return
    }
    MasterSenderInfo.Username = user.Username
    return
}

// service needs to be at file scope so that handlers can access
var service = new(dripline.AmqpService)

/*
func handler(w http.ResponseWriter, r *http.Request) {
    buf := new(bytes.Buffer)
    buf.ReadFrom(r.Body)
    //fmt.Fprintf(w, "Hi there, I love %s!", r.URL.Path[1:])
    fmt.Fprintf(w, "I received %s!", buf)
}*/

func RequestHandler(w http.ResponseWriter, r *http.Request) {
    /*
    r.ParseForm()  //Parse url parameters passed, then parse the response packet for the POST body (request body)
    // attention: If you do not call ParseForm method, the following data can not be obtained form
    logging.Log.Infof("new request........")
    logging.Log.Debugf("Form:\n%v", r.Form) // print information on server side.
    logging.Log.Debugf("path: %v", r.URL.Path)
    logging.Log.Debugf("scheme: %v", r.URL.Scheme)
    logging.Log.Debugf("URL long: %v", r.Form["url_long"])
    for k, v := range r.Form {
        logging.Log.Debugf("key: %v", k)
        logging.Log.Debugf("val: %v", strings.Join(v, ""))
    }
    logging.Log.Debugf("parsing further")
    */

    // I'm not sure what this is for, the above seemed useful
    /*reqDump, rdErr := httputil.DumpRequest(r, true)
    if rdErr == nil {
        fmt.Fprintf(w, "Request received: %q", reqDump)
    } else {
        http.Error(w, fmt.Sprint(rdErr), http.StatusInternalServerError)
    }*/

    /*
    type FooStr struct {
        MsgType dripline.MsgCodeT `json:"msgtype"`
        TimeStamp string `json:"timestamp"`
        SenderInfo dripline.SenderInfo `json:"sender_info"`
    }
    */

    logging.Log.Notice("\ndecoding new request")
    decoder := json.NewDecoder(r.Body)
    //var reqMessage FooStr
    var reqMessage dripline.Request
    err := decoder.Decode(&reqMessage)
    if err == nil {
        logging.Log.Debugf("json decoded")
        logging.Log.Debugf("object is:\n%v", reqMessage)
        logging.Log.Debugf("msgtype is: %v", reqMessage.MsgType)
        logging.Log.Debugf("target is: %v", reqMessage.Target)
        logging.Log.Debugf("sender info is:\n%v", reqMessage.SenderInfo)
        logging.Log.Debugf("operation is: %v", reqMessage.MsgOp)
    } else {
        logging.Log.Debugf("json decoder fail with: %v", err)
    }
    // send the decoded message
    var timeOut time.Duration
    timeOut = time.Duration(60)*time.Second
    _,e := service.SendRequest(reqMessage, timeOut)
    if e == nil {
    } else {
        logging.Log.Debug("sending request failed with: %v", e)
    }

    return
}

func AlertHandler(w http.ResponseWriter, r *http.Request) {
    reqDump, rdErr := httputil.DumpRequest(r, true)
    if rdErr == nil {
        fmt.Fprintf(w, "Alert received: %q", reqDump)
    } else {
        http.Error(w, fmt.Sprint(rdErr), http.StatusInternalServerError)
    }
    return
}

func main() {
    logging.InitializeLogging()

    // user needs help
    var needHelp bool

    // configuration file
    var configFile string

    // set up flag to point at conf, parse arguments and then verify
    flag.BoolVar(&needHelp,
        "help",
        false,
        "Display this dialog")
    flag.StringVar(&configFile,
        "config",
        "",
        "JSON configuration file")
    flag.Parse()

    if needHelp {
        flag.Usage()
        os.Exit(1)
    }

    // defult configuration
    viper.SetDefault("log-level", "DEBUG")
    viper.SetDefault("broker", "localhost")
    viper.SetDefault("queue", "http2dripline")

    // load config
    if configFile != "" {
        viper.SetConfigFile(configFile)
        if parseErr := viper.ReadInConfig(); parseErr != nil {
        	logging.Log.Criticalf("%v", parseErr)
        	os.Exit(1)
        }
        logging.Log.Notice("Config file loaded")
    }
    logging.ConfigureLogging(viper.GetString("log-level"))
    logging.Log.Infof("Log level: %v", viper.GetString("log-level"))

    broker := viper.GetString("broker")
    queueName := viper.GetString("queue")

    // check authentication for desired username
    if authErr := authentication.Load(); authErr != nil {
        logging.Log.Criticalf("Error in loading authenticators: %v", authErr)
        os.Exit(1)
    }

    if ! authentication.AmqpAvailable() {
        logging.Log.Critical("Authentication for AMQP is not available")
        os.Exit(1)
    }

    amqpUser := authentication.AmqpUsername()
    amqpPassword := authentication.AmqpPassword()

    url := "amqp://" + amqpUser + ":" + amqpPassword + "@" + broker
    logging.Log.Debugf("AMQP URL: %v", url)

    // recall that service is at global scope for this file
    service = dripline.StartService(url, queueName)
    if (service == nil) {
        logging.Log.Critical("AMQP service did not start")
        os.Exit(1)
    }
    logging.Log.Info("AMQP service started")

    // add .# to the queue name for the subscription 
    subscriptionKey := queueName + ".#"
    if subscribeErr := service.SubscribeToRequests(subscriptionKey); subscribeErr != nil {
        logging.Log.Criticalf("Could not subscribe to requests at <%v>: %v", subscriptionKey, subscribeErr)
        os.Exit(1)
    }

    if msiErr := fillMasterSenderInfo(); msiErr != nil {
        logging.Log.Criticalf("Could not fill out master sender info: %v", MasterSenderInfo)
        os.Exit(1)
    }


    logging.Log.Info("Starting server")

    http.HandleFunc("/request", RequestHandler)
    http.HandleFunc("/alert", AlertHandler)
    http.ListenAndServe(":8080", nil)
}
