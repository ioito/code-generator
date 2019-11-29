package generators

import (
	"fmt"
	"path/filepath"
	"strings"

	"k8s.io/gengo/generator"
	"k8s.io/gengo/types"

	//"yunion.io/x/log"
	"yunion.io/x/pkg/utils"
)

func privateName(structName, methodName string) string {
	if structName == "" {
		return methodName
	}
	return structName + "_" + methodName
	//if len(structName) == 0 {
	//return methodName
	//}
	//firstStr := strings.ToLower(string(structName[0]))
	//if len(structName) == 1 {
	//return firstStr + "_" + methodName
	//}
	//restStr := structName[1:len(structName)]
	//return firstStr + restStr + "_" + methodName
}

type routeFactory struct {
	method *Method
}

func newRouteFactory(m *Method) *routeFactory {
	return &routeFactory{method: m}
}

func (f *routeFactory) apiAction(trimPrefix string) string {
	method := f.method
	apiAction := utils.CamelSplit(strings.TrimPrefix(method.Name(), trimPrefix), "-")
	return apiAction
}

func (f *routeFactory) newRoute(action string, input *parameter, output *response) *route {
	method := f.method
	r := &route{
		action:    action,
		parameter: input,
		tags:      []string{method.resSingular},
		response: map[int]*response{
			200: output,
		},
	}
	commentLines := method.Method().CommentLines
	if len(commentLines) > 0 {
		r.summary = commentLines[0]
	}
	if len(input.errorMsgs) != 0 || len(output.errorMsgs) != 0 {
		r.summary = "input or output error exists"
	}
	desc := make([]string, 0)
	if len(input.errorMsgs) != 0 {
		desc = append(desc, fmt.Sprintf("input error: %s", strings.Join(input.errorMsgs, ",")))
	}
	if len(output.errorMsgs) != 0 {
		desc = append(desc, fmt.Sprintf("output error: %s", strings.Join(output.errorMsgs, ",")))
	}
	if len(commentLines) > 1 {
		desc = append(desc, commentLines[1:len(commentLines)]...)
	}
	r.description = desc
	r.reviseDescription()
	return r
}

func (r *route) reviseDescription() {
	if r.summary != "" && len(r.description) == 0 {
		r.description = append(r.description, r.summary)
	}
}

func (f *routeFactory) Create(input *parameter, output *response) *route {
	r := f.newRoute("POST", input, output)
	r.path = fmt.Sprintf("/%s", f.method.resPlural)
	return r
}

func (f *routeFactory) List(input *parameter, output *response) *route {
	r := f.newRoute("GET", input, output)
	r.path = fmt.Sprintf("/%s", f.method.resPlural)
	return r
}

func (f *routeFactory) Get(input *parameter, output *response) *route {
	r := f.newRoute("GET", input, output)
	r.path = fmt.Sprintf("/%s/{id}", f.method.resPlural)
	return r
}

func (f *routeFactory) Update(input *parameter, output *response) *route {
	r := f.newRoute("PUT", input, output)
	r.path = fmt.Sprintf("/%s/{id}", f.method.resPlural)
	return r
}

func (f *routeFactory) Delete(input *parameter, output *response) *route {
	r := f.newRoute("DELETE", input, output)
	r.path = fmt.Sprintf("/%s/{id}", f.method.resPlural)
	return r
}

func (f *routeFactory) GetSpec(input *parameter, output *response) *route {
	apiAction := f.apiAction(GetSpec)
	r := f.newRoute("GET", input, output)
	r.path = fmt.Sprintf("/%s/{id}/%s", f.method.resPlural, apiAction)
	return r
}

func (f *routeFactory) PerformAction(input *parameter, output *response) *route {
	apiAction := f.apiAction(Perform)
	r := f.newRoute("POST", input, output)
	r.path = fmt.Sprintf("/%s/{id}/%s", f.method.resPlural, apiAction)
	return r
}

type route struct {
	// action means restful GET, POST, PUT, DELETE
	action      string
	path        string
	parameter   *parameter
	tags        []string
	summary     string
	description []string
	response    map[int]*response
}

func (r route) Do(sw *generator.SnippetWriter) {
	sw.Do(fmt.Sprintf(
		"// swagger:route %s %s %s %s\n",
		r.action,
		r.path,
		strings.Join(r.tags, " "),
		r.parameter.operationId,
	), nil)
	h := newSW(sw)
	if len(r.summary) != 0 {
		h.emptyLine()
		h.line(r.summary)
	}
	if len(r.description) != 0 {
		h.emptyLine()
		h.lines(r.description)
	}
	h.emptyLine()
	h.line("responses:")
	// TODO: responses for errors
	for code, resp := range r.response {
		h.line(fmt.Sprintf("%d: %s", code, resp.id))
	}
	sw.Do("\n", nil)
}

type paramterFactory struct {
	method *Method
}

func newParameterFactory(m *Method) *paramterFactory {
	return &paramterFactory{method: m}
}

func (f *paramterFactory) newParameter() *parameter {
	p := newParameter(
		f.method.resSingular, f.method.resPlural,
		privateName(f.method.resSingular, f.method.Name()))
	return p
}

func (f *paramterFactory) Create() *parameter {
	// pattern: func(ctx, userCred, ownerId, query, data)
	query := f.method.Params(3)
	body := f.method.Params(4)
	p := f.newParameter()
	if isStructPointer(body) {
		p.body = body
	} else {
		p.errorMsgs = append(p.errorMsgs, fmt.Sprintf("unsupport body type %s", body.String()))
	}
	p.query = query
	return p
}

func (f *paramterFactory) List() *parameter {
	// pattern: func(ctx, q, userCred, query)
	query := f.method.Params(3)
	p := f.newParameter()
	if isStructPointer(query) {
		p.query = query.Elem
	} else {
		p.errorMsgs = append(p.errorMsgs, fmt.Sprintf("unsupport query type %s", query.String()))
	}
	return p
}

func (f *paramterFactory) Get() *parameter {
	// pattern: func(ctx, userCred, query)
	query := f.method.Params(2)
	p := f.newParameter()
	if isStructPointer(query) {
		p.query = query.Elem
	}
	p.withId = true
	return p
}

func (f *paramterFactory) Update() *parameter {
	// pattern: func(ctx, userCred, query, data)
	query := f.method.Params(2)
	body := f.method.Params(3)
	p := f.newParameter()
	if isStructPointer(body) {
		p.body = body
	} else {
		p.errorMsgs = append(p.errorMsgs, fmt.Sprintf("unsupport body type %s", body.String()))
	}
	p.query = query
	p.withId = true
	return p
}

func (f *paramterFactory) Delete() *parameter {
	// pattern: func(ctx, userCred, query, data)
	query := f.method.Params(2)
	body := f.method.Params(3)
	p := f.newParameter()
	if isStructPointer(query) {
		p.query = query
	}
	if isStructPointer(body) {
		p.body = body
	}
	p.withId = true
	return p
}

func (f *paramterFactory) GetSpec() *parameter {
	// pattern: func(ctx, userCred, query)
	query := f.method.Params(2)
	p := f.newParameter()
	if isStructPointer(query) {
		p.query = query.Elem
	}
	p.withId = true
	return p
}

func (f *paramterFactory) PerformAction() *parameter {
	// pattern: func(ctx, userCred, query, body)
	query := f.method.Params(2)
	body := f.method.Params(3)
	p := f.newParameter()
	if isStructPointer(body) {
		p.body = body
	}
	p.query = query
	p.withId = true
	return p
}

type parameter struct {
	singular    string
	plural      string
	operationId string
	withId      bool
	query       *types.Type
	body        *types.Type

	errorMsgs []string
}

func newParameter(singular, plural, id string) *parameter {
	return &parameter{
		singular:    singular,
		plural:      plural,
		operationId: id,
		errorMsgs:   make([]string, 0),
	}
}
func (r parameter) Do(sw *generator.SnippetWriter) {
	h := newSW(sw)
	h.line(fmt.Sprintf("swagger:parameters %s", r.operationId))
	r.do(sw, h)
}

func getValidType(t *types.Type) *types.Type {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case types.Pointer:
		return t.Elem
	case types.Struct:
		return t
	default:
		return nil
	}
}

func GetValidType(t *types.Type) *types.Type {
	t = getValidType(t)
	if t == nil {
		return nil
	}
	if strings.Contains(t.Name.Package, "yunion.io/x/jsonutils") {
		return nil
	}
	return t
}

func (r parameter) getQuery() *types.Type {
	return GetValidType(r.query)
}

func (r parameter) getBody() *types.Type {
	return GetValidType(r.body)
}

func (r parameter) do(sw *generator.SnippetWriter, h *snippetWriter) {
	sw.Do(fmt.Sprintf("type %s struct {\n", r.operationId), nil)
	if r.withId {
		h.line(fmt.Sprintf("The Id or Name of %s", r.singular))
		h.line("in:path")
		h.line("required:true")
		sw.Do("Id string `json:\"id\"`\n", nil)
	}
	query := r.getQuery()
	if query != nil {
		args := getArgs(query)
		sw.Do("$.type|raw$\n", args)
	}
	body := r.getBody()
	if r.body != nil {
		args := getArgs(body)
		sw.Do("// in:body\n", nil)
		if r.singular != "" {
			sw.Do("Body struct {", nil)
			sw.Do(fmt.Sprintf("Input $.type|raw$ `json:\"%s\"`\n", r.singular), args)
			sw.Do("} `json:\"body\"`", nil)
			//sw.Do(fmt.Sprintf("Body $.type|raw$ `json:\"%s\"`\n", r.singular), args)
		} else {
			sw.Do("Body $.type|raw$ `json:\"body\"`", args)
		}
	}
	sw.Do("}\n", nil)
}

type responseFactory struct {
	method *Method
}

type response struct {
	output  *types.Type
	id      string
	bodyKey string
	isList  bool

	errorMsgs []string
}

func (r response) getOutput() *types.Type {
	return GetValidType(r.output)
}

func (r response) Do(sw *generator.SnippetWriter) {
	h := newSW(sw)
	sw.Do(fmt.Sprintf("// swagger:response %s\n", r.id), nil)
	sw.Do(fmt.Sprintf("type %s struct {\n", r.id), nil)
	output := r.getOutput()
	args := getArgs(output)
	if output != nil {
		h.line("in:body")
		if r.bodyKey != "" {
			r.bodyStruct(output, sw)
		} else {
			sw.Do("$.type|raw$", args)
		}
	}
	sw.Do("}\n", nil)
}

func (r response) bodyStruct(output *types.Type, sw *generator.SnippetWriter) {
	args := getArgs(output)
	sw.Do("Body struct {\n", nil)
	if r.isList {
		sw.Do(fmt.Sprintf("Output []$.type|raw$ `json:\"%s\"`\n", r.bodyKey), args)
		sw.Do("Limit int `json:\"limit\"`\n", nil)
		sw.Do("Total int `json:\"total\"`\n", nil)
		sw.Do("Offset int `json:\"offset\"`\n", nil)
	} else {
		sw.Do(fmt.Sprintf("Output $.type|raw$ `json:\"%s\"`\n", r.bodyKey), args)
	}
	sw.Do("}\n", nil)
}

func newResponseFactory(m *Method) *responseFactory {
	return &responseFactory{method: m}
}

func (f *responseFactory) newResponse() *response {
	return &response{
		id:        fmt.Sprintf("%sOutput", privateName(f.method.resSingular, f.method.Name())),
		errorMsgs: make([]string, 0),
	}
}

func (f *responseFactory) FirstSingularResult() *response {
	// return pattern: ObjectPtr, error
	return f.ResultByMethod(f.method, 0, f.method.resSingular)
}

func (f *responseFactory) FirstSingularResultNoError() *response {
	// return pattern: ObjectPtr, error
	return f.resultByMethod(f.method, 0, f.method.resSingular, true)
}

func (f *responseFactory) resultByMethod(method *Method, resultIdx int, bodyKey string, ignoreErr bool) *response {
	r := f.newResponse()
	sig := method.Signature()
	params := sig.Results
	out := params[resultIdx]
	if isStructPointer(out) {
		r.output = out
	} else if !ignoreErr {
		r.errorMsgs = append(r.errorMsgs, fmt.Sprintf("unsupport type %s", out.String()))
	}
	if bodyKey == "" {
		bodyKey = f.method.resPlural
	}
	r.bodyKey = bodyKey
	return r
}

func (f *responseFactory) ResultByMethod(method *Method, resultIdx int, bodyKey string) *response {
	return f.resultByMethod(method, resultIdx, bodyKey, false)
}

func (f *responseFactory) ResultByGetMethod(getMethod *Method) *response {
	return f.ResultByMethod(getMethod, 0, f.method.resSingular)
}

func (f *responseFactory) ListResult(getMethod *Method) *response {
	r := f.ResultByMethod(getMethod, 0, f.method.resPlural)
	r.isList = true
	return r
}

type SwaggerConfigRoute struct {
	Method string
	Path   string
	Tags   []string
}

func (c *SwaggerConfigRoute) newRoute(input *parameter, output *response) *route {
	r := &route{
		action:    c.Method,
		path:      c.Path,
		parameter: input,
		tags:      c.Tags,
		response: map[int]*response{
			200: output,
		},
	}
	return r
}

type SwaggerConfigParam struct {
	Body  *types.Type
	Query *types.Type
}

func (c *SwaggerConfigParam) newParameter(t *types.Type) *parameter {
	n := filepath.Base(t.Name.Package)
	param := newParameter("", "", privateName(n, t.Name.Name))
	param.query = c.Query
	param.body = c.Body
	return param
}

type SwaggerConfigResponse struct {
	Output  *types.Type
	BodyKey string
}

func (c *SwaggerConfigResponse) newResponse(t *types.Type) *response {
	n := filepath.Base(t.Name.Package)
	r := &response{
		id:        fmt.Sprintf("%sOutput", privateName(n, t.Name.Name)),
		bodyKey:   c.BodyKey,
		errorMsgs: make([]string, 0),
	}
	r.output = c.Output
	return r
}

type SwaggerConfig struct {
	Route    *SwaggerConfigRoute
	Param    *SwaggerConfigParam
	Response *SwaggerConfigResponse
}

func (c *SwaggerConfig) generate(t *types.Type, sw *generator.SnippetWriter) {
	param := c.Param.newParameter(t)
	resp := c.Response.newResponse(t)
	route := c.Route.newRoute(param, resp)
	commentLines := t.CommentLines
	if len(commentLines) > 0 {
		route.summary = commentLines[0]
	}
	desc := make([]string, 0)
	if len(commentLines) > 1 {
		desc = append(desc, commentLines[1:len(commentLines)]...)
	}
	route.description = desc
	route.reviseDescription()

	cc := &commenter{
		route:     route,
		parameter: param,
		response:  resp,
	}
	cc.Do(sw)
}
