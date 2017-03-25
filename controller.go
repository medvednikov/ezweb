// controller.go

package gomvc

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

// Controller is the core type of gomvc
type Controller struct {
	Request *http.Request
	Out     http.ResponseWriter

	// Params is a map of all query string key-vales:
	// example.com/?a=1&b=2 => map[string]string{ "a":"1", "b":"2" }
	Params map[string]string

	// Form is a map with form values submitted by a POST request
	Form map[string]string

	// Uri contains current path:
	// example.com/Account/Unsubscribe?email=1 => "Account/Unsubscribe"
	Uri string

	// ActionName is the name of the running action (method)
	ActionName string

	// ControllerName is the name of the controller subtype
	ControllerName string

	// CustomTemplateFuncs defines extra html/template functions that can
	// be run in all html templates used in this controller
	CustomTemplateFuncs template.FuncMap

	// PageTitle defines the title of the HTML page and is set in the action
	PageTitle string

	FlashMsg string

	gorillaSession *sessions.Session
	Session        map[string]string

	stopped bool
}

// View executes a template corresponding to the current controller method
func (c *Controller) Render(data interface{}) {
	if c.stopped {
		return
	}
	c.cleanUp()
	t := template.New("root").
		Delims(config.DelimLeft, config.DelimRight).
		Funcs(defaultFuncs).
		Funcs(c.CustomTemplateFuncs)
	// A helper function for logging errors
	showError := func(msg string, err error) {
		log.Println(msg, err)
		if config.IsDev {
			c.Write(msg, err)
		}
	}
	// Parse layout file with all subtemplates first
	_, err := t.New("layout.html").Parse(readTemplate("layout.html"))
	if err != nil {
		showError("Layout template parsing error", err)
		return
	}
	// Parse the local layout template
	localLayout := c.ControllerName + "/_layout.html"
	_, err = t.New(localLayout).Parse(readTemplate(localLayout))
	if err != nil {
		showError("Local layout template parsing error", err)
		return
	}
	// Now parse the actual template file corresponding to the action
	path := c.ControllerName + "/" + stripMethodType(c.ActionName) + ".html"
	_, err = t.New(path).Parse(readTemplate(path))
	if err != nil {
		showError("Template parsing error", err)
		return
	}
	// Finally, execute it
	err = t.ExecuteTemplate(c.Out, path, data)
	if err != nil {
		log.Println("Template execution error:", err)
		if config.IsDev {
			c.Write("Template execution error:", err)
		}
		return
	}
}

// Say prints a string with a newline to http response
func (c *Controller) Say(s ...interface{}) {
	fmt.Fprint(c.Out, s...)
	fmt.Fprintln(c.Out)
}

// Write prints a string to http response
func (c *Controller) Write(s ...interface{}) {
	fmt.Fprint(c.Out, s...)
}

// EmptyHandler returns an empty handler for http.HandleFunc
// This is used to explicitely leave certain routes unprocessed.
func EmptyHandler(w http.ResponseWriter, r *http.Request) {

}

// GetCookie returns a value of the cookie with a specified key.
// If no such cookie was found, an empty string is returned.
func (c *Controller) GetCookie(key string) string {
	res, _ := c.Request.Cookie(key)
	if res == nil {
		return ""
	}
	return res.Value
}

// SetCookie creates a new cookie valid for 10 days
func (c *Controller) SetCookie(key string, value string) {
	http.SetCookie(c.Out, &http.Cookie{
		Name:    key,
		Value:   value,
		Expires: time.Now().Add(10 * 24 * time.Hour),
		Path:    "/",
	})
}

func (c *Controller) DeleteCookie(key string) {
	http.SetCookie(c.Out, &http.Cookie{Name: key, Value: "", MaxAge: -1})
}

func (c *Controller) SetContentType(ct string) {
	c.Out.Header().Set("Content-Type", ct)
}

func (c *Controller) AllowOrigin(val string) {
	c.Out.Header().Set("Access-Control-Allow-Origin", val)
}

func (c *Controller) SetHeader(header, value string) {
	c.Out.Header().Set(header, value)
}

func (c *Controller) IsAjax() bool {
	h := c.Request.Header["X-Requested-With"]
	return len(h) > 0 && h[0] == "XMLHttpRequest"
}

func (c *Controller) RenderError(msg string, code int) {
	c.cleanUp()
	http.Error(c.Out, msg, code)
	c.stopped = true
}

func (c *Controller) Flash(s string) {
	c.FlashMsg = s
	c.Session["gomvc_flash"] = s
}

// Abort stops execution of the current action immediately
func (c *Controller) Abort() {
	c.stopped = true
}

// IP returns client's IP address
func (c *Controller) IP() string {
	return c.Request.Header.Get("X-Forwarded-For")
}

// renderJson returns a marshaled json object with content type 'application/json'.
// This is usually used for responding to AJAX requests.
func (c *Controller) renderJson(model interface{}) {
	if c.stopped {
		return
	}
	c.cleanUp()
	c.SetContentType("application/json")
	obj, err := json.MarshalIndent(model, "", "\t")
	if err != nil {
		log.Println(err)
		return
	}
	c.Write(string(obj))
}

// Index defines a default action
func (c *Controller) Index() {
	c.Say(`Welcome to gomvc! Define your own Index action:

    type Home struct {
        *gomvc.Controller
    }

    func (c *Home) Index() {
        c.Write("Hello world!")
    }
    `)
}

// InitValues parses the http.Request object and fetches all necessary values
// for gomvc.Controller
func (c *Controller) InitValues(w http.ResponseWriter, r *http.Request) {
	c.Out = w
	c.Request = r
	values := r.URL.Query()
	c.Uri = r.URL.Path[1:]
	c.ActionName = getActionFromUri(c.Uri, c.ControllerName)
	// ActionPOST, ActionDELETE etc
	if r.Method != "GET" {
		c.ActionName += r.Method
	}
	c.PageTitle = ""
	// Generate query string map (Params)
	c.Params = make(map[string]string)
	for key, _ := range values {
		c.Params[strings.ToLower(key)] = values.Get(key)
	}
	// Assign routing variables to Params
	for key, value := range mux.Vars(r) {
		c.Params[strings.ToLower(key)] = value
	}
	// Generate form data
	c.Form = make(map[string]string)
	c.Request.ParseForm()
	for key, _ := range c.Request.PostForm {
		c.Form[strings.ToLower(key)] = c.Request.PostForm.Get(key)
	}
	// Session
	c.gorillaSession, _ = cookieStore.Get(c.Request, config.SessionID)
	c.Session = make(map[string]string, 0)
	for key, val := range c.gorillaSession.Values {
		c.Session[fmt.Sprintf("%v", key)] = fmt.Sprintf("%v", val)
	}
	c.FlashMsg = c.Session["gomvc_flash"]
}

func (c *Controller) checkMethodType() bool {
	types := []string{"POST", "PUT", "DELETE"}
	for _, t := range types {
		if strings.Index(c.ActionName, t) > -1 &&
			c.Request.Method != t {
			c.Write(t, " expected")
			return false
		}
	}
	return true
}

// runMethod runs a specified controller action (method)
func runMethod(method reflect.Value, c *Controller) {
	if !method.IsValid() {
		http.NotFound(c.Out, c.Request)
		if config.IsDev {
			c.Write("Unknown action '" + c.ActionName +
				"' (controller: '" + c.ControllerName + "')")
		}
		return
	}
	if !c.checkMethodType() {
		return
	}
	if c.stopped {
		return
	}
	// Run it via reflect
	values := make([]reflect.Value, 0)
	// Loop thru all method args and assign query string parameters to them
	for i, argName := range ActionArgs[c.ControllerName][c.ActionName] {
		// Get value from the query string (params)
		// Register(name, password string) => /Register?name=a;password=b
		stringValue := c.Params[strings.ToLower(argName)]
		// Convert this argument to a value of a certain type (Form,
		// string, int)
		argType := method.Type().In(i)
		values = append(values, c.argToValue(stringValue, argType))
	}
	// TODO handle empty values
	//fmt.Println(c.ControllerName, c.ActionName, values, dump(ActionArgs))
	results := method.Call(values)
	if len(results) > 0 {
		res := results[0].Interface()
		switch res.(type) {
		case JSON:
			c.renderJson(res.(JSON).Model)
		case View:
			switch res.(View).Model.(type) {
			case RedirectResult:
			default:
				c.Render(res.(View).Model)
			}
		case string:
			c.Write(res.(string))
		}
	}
}

// argToValue generates a reflect.Value from an argument type and its
// corresponding query string or form value
func (c *Controller) argToValue(stringValue string, argType reflect.Type) reflect.Value {
	// Handle a struct pointer, this must be a form
	if argType.Kind() == reflect.Ptr && argType.Elem().Kind() == reflect.Struct {
		// Dereference the form
		argType = argType.Elem()
		// Create a new form object
		newFormObj := reflect.New(argType)
		// Set all its fields
		for i := 0; i < argType.NumField(); i++ {
			field := newFormObj.Elem().Field(i)
			fieldName := argType.Field(i).Name // e.g. "Id", "Title"
			formValue := c.Form[strings.ToLower(fieldName)]
			switch field.Type().Name() {
			case "int":
				field.SetInt(int64(toint(formValue)))
			case "float64":
				field.SetFloat(tofloat(formValue))
			case "string":
				field.SetString(formValue)
			}
		}
		return newFormObj
	} else if argType.Name() == "int" {
		// Convert to int if this argument is an int, otherwise leave
		// it as a string TODO more types?
		return reflect.ValueOf(toint(stringValue))
	}
	return reflect.ValueOf(stringValue)
}

func (c *Controller) cleanUp() {
	// Save session
	for key, val := range c.Session {
		c.gorillaSession.Values[key] = val
	}
	c.gorillaSession.Save(c.Request, c.Out)
}
