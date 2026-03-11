package ir

// Program is the normalized intermediate representation used by the IR backend.
type Program struct {
	TopLevel  []TopLevelNode
	Server    *Server
	Routes    []Route
	Groups    []Group
	Functions []Function
	Hooks     Hooks
	Errors    []ErrorHandler
	Schedules []Schedule
	Features  Features
}

type Server struct {
	HasServerBlock bool
	Port           int
	HasTLS         bool
	HasAutocert    bool
	HasSession     bool
	HasTemplates   bool
}

type Route struct {
	Method        string
	Path          string
	TypeCheck     string
	Timeout       int
	CSRFDisabled  bool
	HasElse       bool
	HasDisconnect bool
	BodyPreview   []string
	ElsePreview   []string
	DiscPreview   []string
}

type Group struct {
	Prefix      string
	RouteCount  int
	BeforeCount int
	AfterCount  int
}

type Function struct {
	Name        string
	Params      []string
	BodyPreview []string
}

type Hooks struct {
	InitCount     int
	BeforeCount   int
	AfterCount    int
	ShutdownCount int
}

type ErrorHandler struct {
	StatusCode int
}

type Schedule struct {
	Kind     string // "interval" | "cron"
	Interval int
	CronExpr string
}

type Features struct {
	HasCron    bool
	HasSSE     bool
	HasSession bool
	HasCSRF    bool
	HasDB      bool
	HasMongo   bool
	HasSQL     bool
	HasExec    bool
	HasTLS     bool
	DBDrivers  map[string]bool
}

type TopLevelKind string

const (
	TopLevelUnknown  TopLevelKind = "unknown"
	TopLevelRoute    TopLevelKind = "route"
	TopLevelFunction TopLevelKind = "function"
	TopLevelServer   TopLevelKind = "server"
	TopLevelGroup    TopLevelKind = "group"
	TopLevelBefore   TopLevelKind = "before"
	TopLevelAfter    TopLevelKind = "after"
	TopLevelInit     TopLevelKind = "init"
	TopLevelShutdown TopLevelKind = "shutdown"
	TopLevelHelp     TopLevelKind = "help"
	TopLevelError    TopLevelKind = "error"
	TopLevelEvery    TopLevelKind = "every"
)

type TopLevelNode struct {
	Kind   TopLevelKind
	Line   int
	Column int
}
