package engine

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/upper/db/v4"
	"github.com/upper/db/v4/adapter/sqlite"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	// SQLite3 DB Driver
	_ "github.com/mattn/go-sqlite3"
)

const sqliteDBFile = "./wfb.db"

type Engine struct {
	debug               bool
	portWebhookListener string

	matrixhomeserver string
	matrixusername   string
	matrixpassword   string

	db db.Session

	workflows map[uint64]*workflow
	triggers  map[string]map[string]Trigger

	client *mautrix.Client
}

type RunParams struct {
	Debug               bool
	PortWebhookListener string
	MatrixHomeServer    string
	MatrixUsername      string
	MatrixPassword      string
}

func (e *Engine) StartUp() {
	e.log("Starting up engine..")

	// Initialize maps
	e.workflows = make(map[uint64]*workflow)
	e.triggers = make(map[string]map[string]Trigger)
	e.triggers["webhook"] = make(map[string]Trigger)
	e.triggers["poll"] = make(map[string]Trigger)

	// Establish database connection
	e.log("Attempting to establish database connection..")
	err := e.loadDB()
	if err != nil {
		log.Fatal(err)
	}

	// Load registered workflows from the database and initialize the right triggers for them
	e.log("Loading data...")
	e.loadData()

	// Start Matrix client
	// Note: Matrix client needs to be initialized early as a trigger can try to run Matrix related tasks
	e.log("Starting up Matrix client..")
	go func() {
		err = e.initMatrixClient()
		if err != nil {
			log.Fatal(err)
		}
		e.log("Finished loading Matrix client..")
	}()

	// allow the matrix client to sync and be ready,
	// before we invoke run() on Engine
	time.Sleep(time.Second * 5)

	e.log("Finished starting up engine.")
}

func (e *Engine) ShutDown() {
	// Close database connection
	e.db.Close()
}

func (e *Engine) Run() {
	e.log("\nAt last, running the engine now..")

	go e.runPoller()

	e.runWebhookListener()
}

func (e *Engine) log(m string) {
	if e.debug {
		fmt.Println(m)
	}
}

func (e *Engine) loadDB() (err error) {
	database, err := sql.Open("sqlite3", sqliteDBFile)
	if err != nil {
		log.Fatalf("db.Open(): %q\n", err)
	}
	defer database.Close()

	// Run DB migration
	driver, err := sqlite3.WithInstance(database, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("creating sqlite3 db driver failed %s", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://migration/", "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("initializing db migration failed %s", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrating database failed %s", err)
	}

	// Use upper.io ORM now
	e.db, err = sqlite.Open(sqlite.ConnectionURL{
		Database: sqliteDBFile,
	})
	if err != nil {
		log.Fatalf("db.Open(): %q\n", err)
	}

	// Set database logging to Errors only when debug:false
	if !e.debug {
		db.LC().SetLevel(db.LogLevelError)
	}

	return
}

func (e *Engine) registerWebhookTrigger(t *webhookt) {
	// Add engine instance to inside of trigger, required for starting workflows
	t.engine = e

	// Let the engine know what urlSuffix are associated with this particular instance of trigger
	e.triggers["webhook"][t.urlSuffix] = t

	e.log(fmt.Sprintf("> Registered webhook trigger: %s (urlSuffix: %s)", t.name, t.urlSuffix))
}

func (e *Engine) registerPollTrigger(t *pollt) {
	// Add engine instance to inside of trigger, required for starting workflows
	t.engine = e

	e.log(fmt.Sprintf("> Registered poll trigger: %s (pollingInterval: %s)", t.name, t.pollingInterval))
}

func (e *Engine) loadData() {
	// load triggers & registers them first
	for _, t := range getConfiguredTriggers(e.db) {
		switch t := t.(type) {
		case *webhookt:
			e.registerWebhookTrigger(t)
		case *pollt:
			e.registerPollTrigger(t)
		}
	}

	// load workflows
	for _, w := range getConfiguredWorkflows(e.db) {
		// copy over the value in a separate variable because we need to store a pointer
		// w gets assigned a different value with every iteration, which modifies all values if address of w is taken directly
		instance := w
		e.workflows[w.id] = &instance
	}

	// load workflow steps
	for _, ws := range getConfiguredWFSteps(e.db) {
		switch ws := ws.(type) {
		case *postMessageMatrixWorkflowStep:
			e.workflows[ws.workflow_id].addWorkflowStep(ws)
		case *sendEmailWorkflowStep:
			e.workflows[ws.workflow_id].addWorkflowStep(ws)
		}
	}
}

func (e *Engine) runWebhookListener() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		e.log(fmt.Sprintf("Request received on webhook listener! %s", r.URL.Path))

		if !strings.HasPrefix(r.URL.Path, "/webhooks-listener/") {
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}

		suffix := strings.TrimSuffix(
			strings.TrimPrefix(
				r.URL.Path,
				"/webhooks-listener/",
			),
			"/",
		)

		t, exists := e.triggers["webhook"][suffix]
		e.log(fmt.Sprintf("suffix: %s registered: %t", suffix, exists))
		if exists {
			// @TODO:
			// figure out what data do we have here
			// check in this order:
			// GET request
			// 		message param unless specified param=m, in which case read 'm' param
			// POST request
			//		data param

			keys, ok := r.URL.Query()["message"]
			if !ok || len(keys[0]) < 1 {
				http.Error(w, "400 bad request.", http.StatusBadRequest)
				return
			}

			t.process(keys[0])
			// t.process()
		} else {
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}
	})

	e.log(fmt.Sprintf("> Starting webhook listener at port %s...", e.portWebhookListener))
	if err := http.ListenAndServe(":"+e.portWebhookListener, nil); err != nil {
		log.Fatal(err)
	}
}

func (e *Engine) runPoller() {
	e.log("> Running polls...")
	for _, t := range e.triggers["poll"] {
		go func(t Trigger) {
			t.setup()
		}(t)
	}
}

func (e *Engine) initMatrixClient() (err error) {
	if e.debug {
		fmt.Println("Logging into", e.matrixhomeserver, "as", e.matrixusername)
	}

	e.client, err = mautrix.NewClient(e.matrixhomeserver, "", "")
	if err != nil {
		return
	}
	_, err = e.client.Login(&mautrix.ReqLogin{
		Type:             "m.login.password",
		Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: e.matrixusername},
		Password:         e.matrixpassword,
		StoreCredentials: true,
	})
	if err != nil {
		return
	}

	fmt.Println("Matrix: Login successful!")

	syncer := e.client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
		fmt.Printf("<%[1]s> %[4]s (%[2]s/%[3]s)\n", evt.Sender, evt.Type.String(), evt.ID, evt.Content.AsMessage().Body)
	})

	err = e.client.Sync()
	if err != nil {
		return
	}

	return
}

func NewEngine(p RunParams) *Engine {
	e := Engine{}

	// setting run parameters
	e.debug = p.Debug
	e.portWebhookListener = p.PortWebhookListener
	e.matrixhomeserver = p.MatrixHomeServer
	e.matrixusername = p.MatrixUsername
	e.matrixpassword = p.MatrixPassword

	return &e
}
