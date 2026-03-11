package compiler

// IRProgram is the normalized intermediate representation used by the IR backend.
type IRProgram struct {
	TopLevel  []IRTopLevelNode
	Server    *IRServer
	Routes    []IRRoute
	Groups    []IRGroup
	Functions []IRFunction
	Hooks     IRHooks
	Errors    []IRErrorHandler
	Schedules []IRSchedule
	Features  IRFeatures
}

type IRServer struct {
	HasServerBlock bool
	Port           int
	HasTLS         bool
	HasAutocert    bool
	HasSession     bool
	HasTemplates   bool
}

type IRRoute struct {
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
	Source        *RouteStatement
}

type IRGroup struct {
	Prefix      string
	RouteCount  int
	BeforeCount int
	AfterCount  int
	Source      *GroupStatement
}

type IRFunction struct {
	Name        string
	Params      []string
	BodyPreview []string
	Source      *FnStatement
}

type IRHooks struct {
	InitCount     int
	BeforeCount   int
	AfterCount    int
	ShutdownCount int
}

type IRErrorHandler struct {
	StatusCode int
	Source     *ErrorStatement
}

type IRSchedule struct {
	Kind     string // "interval" | "cron"
	Interval int
	CronExpr string
	Source   *EveryStatement
}

type IRFeatures struct {
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

type IRTopLevelKind string

const (
	IRTopLevelUnknown  IRTopLevelKind = "unknown"
	IRTopLevelRoute    IRTopLevelKind = "route"
	IRTopLevelFunction IRTopLevelKind = "function"
	IRTopLevelServer   IRTopLevelKind = "server"
	IRTopLevelGroup    IRTopLevelKind = "group"
	IRTopLevelBefore   IRTopLevelKind = "before"
	IRTopLevelAfter    IRTopLevelKind = "after"
	IRTopLevelInit     IRTopLevelKind = "init"
	IRTopLevelShutdown IRTopLevelKind = "shutdown"
	IRTopLevelHelp     IRTopLevelKind = "help"
	IRTopLevelError    IRTopLevelKind = "error"
	IRTopLevelEvery    IRTopLevelKind = "every"
)

type IRTopLevelNode struct {
	Kind      IRTopLevelKind
	Statement Statement
}
