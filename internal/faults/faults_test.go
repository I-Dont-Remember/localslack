package faults_test

import (
	"testing"

	"localslack/internal/faults"
)

func TestEmptySetNeverFires(t *testing.T) {
	t.Parallel()

	var i faults.Interceptor = faults.NewSet()
	if o, fired := i.Intercept(t.Context(), faults.Request{Method: "chat.postMessage"}); fired || o.Respond {
		t.Errorf("Intercept = %+v, %v; want no override", o, fired)
	}
}
