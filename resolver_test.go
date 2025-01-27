package huma

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExhaustiveErrors(t *testing.T) {
	type Input struct {
		BoolParam    bool      `query:"bool"`
		IntParam     int       `query:"int"`
		Float32Param float32   `query:"float32"`
		Float64Param float64   `query:"float64"`
		Tags         []int     `query:"tags"`
		Time         time.Time `query:"time"`
		Body         struct {
			Test int `json:"test" minimum:"5"`
		}
	}

	app := newTestRouter()

	app.Resource("/").Get("test", "Test").Run(func(ctx Context, input Input) {
		// Do nothing
	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?bool=bad&int=bad&float32=bad&float64=bad&tags=1,2,bad&time=bad", strings.NewReader(`{"test": 1}`))
	r.Host = "example.com"
	app.ServeHTTP(w, r)

	assert.JSONEq(t, `{"$schema": "https://example.com/schemas/ErrorModel.json", "title":"Unprocessable Entity","status":422,"detail":"Error while processing input parameters","errors":[{"message":"cannot parse boolean","location":"query.bool","value":"bad"},{"message":"cannot parse integer","location":"query.int","value":"bad"},{"message":"cannot parse float","location":"query.float32","value":"bad"},{"message":"cannot parse float","location":"query.float64","value":"bad"},{"message":"cannot parse integer","location":"query[2].tags","value":"bad"},{"message":"unable to validate against schema: invalid character 'b' looking for beginning of value","location":"query.tags","value":"[1,2,bad]"},{"message":"cannot parse time","location":"query.time","value":"bad"},{"message":"Must be greater than or equal to 5","location":"body.test","value":1}]}`, w.Body.String())
}

type Dep1 struct {
	// Only *one* of the following two may be set.
	One string `json:"one,omitempty"`
	Two string `json:"two,omitempty"`
}

func (d *Dep1) Resolve(ctx Context, r *http.Request) {
	if d.One != "" && d.Two != "" {
		ctx.AddError(&ErrorDetail{
			Message:  "Only one of ['one', 'two'] is allowed.",
			Location: "one",
			Value:    d.One,
		})
	}
}

type Dep2 struct {
	// Test recursive resolver with complex input structure containing a map of
	// lists of struct pointers.
	Foo map[string][]*Dep1 `json:"foo"`
}

func TestNestedResolver(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Post("test", "Test",
		NewResponse(http.StatusNoContent, "desc"),
	).Run(func(ctx Context, input struct {
		Body Dep2
	}) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	// Test happy case just sending ONE of the two possible fields in each struct.
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{
		"foo": {
			"a": [{"one": "1"}],
			"b": [{"two": "2"}]
		}
	}`))
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Result().StatusCode)
}

func TestNestedResolverError(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Post("test", "Test",
		NewResponse(http.StatusNoContent, "desc"),
	).Run(func(ctx Context, input struct {
		Body Dep2
	}) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	// Test error case where we send BOTH fields in the same struct, which is
	// not allowed. Should get a validation error response generated by the
	// `Dep1.Resolve(...)` method above.
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{
		"foo": {
			"a": [
				{"one": "1", "two": "2"}
			]
		}
	}`))
	r.Host = "example.com"
	app.ServeHTTP(w, r)

	assert.JSONEq(t, `{
		"$schema": "https://example.com/schemas/ErrorModel.json",
		"status": 422,
		"title": "Unprocessable Entity",
		"detail": "Error while processing input parameters",
		"errors": [
			{
				"message": "Only one of ['one', 'two'] is allowed.",
				"location": "body.foo.a[0].one",
				"value": "1"
			}
		]
	}`, w.Body.String())
}

func TestInvalidJSON(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Post("test", "Test",
		NewResponse(http.StatusNoContent, "desc"),
	).Run(func(ctx Context, input struct {
		Body string
	}) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	// Test happy case just sending ONE of the two possible fields in each struct.
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{.2asdf2`))
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

type QueryParamTestModel struct {
	BooleanParam bool   `query:"b"`
	OtherParam   string `query:"s"`
}

func TestBooleanQueryParamNoVal(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusOK, "desc"),
	).Run(func(ctx Context, input QueryParamTestModel) {
		out := &QueryParamTestModel{
			BooleanParam: input.BooleanParam,
			OtherParam:   input.OtherParam,
		}
		j, err := json.Marshal(out)
		if err == nil {
			ctx.Write(j)
		} else {
			ctx.WriteError(http.StatusBadRequest, "error marshaling to json", err)
		}

	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?s=test&b", nil)
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	decoder := json.NewDecoder(w.Body)
	var o QueryParamTestModel

	err := decoder.Decode(&o)
	if err != nil {
		assert.Fail(t, "Unable to decode json response")
	}

	assert.Equal(t, o.BooleanParam, true)
	assert.Equal(t, o.OtherParam, "test")
}

func TestBooleanQueryParamTrailingEqual(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusOK, "desc"),
	).Run(func(ctx Context, input QueryParamTestModel) {
		out := &QueryParamTestModel{
			BooleanParam: input.BooleanParam,
			OtherParam:   input.OtherParam,
		}
		j, err := json.Marshal(out)
		if err == nil {
			ctx.Write(j)
		} else {
			ctx.WriteError(http.StatusBadRequest, "error marshaling to json", err)
		}

	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?s=test&b=", nil)
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	decoder := json.NewDecoder(w.Body)
	var o QueryParamTestModel

	err := decoder.Decode(&o)
	if err != nil {
		assert.Fail(t, "Unable to decode json response")
	}

	assert.Equal(t, o.BooleanParam, true)
	assert.Equal(t, o.OtherParam, "test")
}

func TestBooleanQueryParamExplicitSet(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusOK, "desc"),
	).Run(func(ctx Context, input QueryParamTestModel) {
		out := &QueryParamTestModel{
			BooleanParam: input.BooleanParam,
			OtherParam:   input.OtherParam,
		}
		j, err := json.Marshal(out)
		if err == nil {
			ctx.Write(j)
		} else {
			ctx.WriteError(http.StatusBadRequest, "error marshaling to json", err)
		}

	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?s=test&b=true", nil)
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	decoder := json.NewDecoder(w.Body)
	var o QueryParamTestModel

	err := decoder.Decode(&o)
	if err != nil {
		assert.Fail(t, "Unable to decode json response")
	}

	assert.Equal(t, o.BooleanParam, true)
	assert.Equal(t, o.OtherParam, "test")
}

func TestBooleanQueryParamNotSet(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusOK, "desc"),
	).Run(func(ctx Context, input QueryParamTestModel) {
		out := &QueryParamTestModel{
			BooleanParam: input.BooleanParam,
			OtherParam:   input.OtherParam,
		}
		j, err := json.Marshal(out)
		if err == nil {
			ctx.Write(j)
		} else {
			ctx.WriteError(http.StatusBadRequest, "error marshaling to json", err)
		}

	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?s=test", nil)
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	decoder := json.NewDecoder(w.Body)
	var o QueryParamTestModel

	err := decoder.Decode(&o)
	if err != nil {
		assert.Fail(t, "Unable to decode json response")
	}

	assert.Equal(t, o.BooleanParam, false)
	assert.Equal(t, o.OtherParam, "test")
}

func TestStringQueryEmpty(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusOK, "desc"),
	).Run(func(ctx Context, input QueryParamTestModel) {
		out := &QueryParamTestModel{
			BooleanParam: input.BooleanParam,
			OtherParam:   input.OtherParam,
		}
		j, err := json.Marshal(out)
		if err == nil {
			ctx.Write(j)
		} else {
			ctx.WriteError(http.StatusBadRequest, "error marshaling to json", err)
		}

	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?s=&b", nil)
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	decoder := json.NewDecoder(w.Body)
	var o QueryParamTestModel

	err := decoder.Decode(&o)
	if err != nil {
		assert.Fail(t, "Unable to decode json response")
	}

	assert.Equal(t, o.BooleanParam, true)
	assert.Equal(t, o.OtherParam, "")
}

func TestRawBody(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusOK, "desc"),
	).Run(func(ctx Context, input struct {
		Body struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		}
		RawBody []byte
	}) {
		ctx.Write(input.RawBody)
	})

	// Note the weird formatting
	body := `{  "name" : "Huma","tags": [ "one"  ,"two"]}`

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/", strings.NewReader(body))
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, body, w.Body.String())

	// Invalid input should still fail validation!
	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodGet, "/", strings.NewReader("{}"))
	app.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Result().StatusCode)
}
