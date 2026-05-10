package plugins

import "testing"

func TestAPISupplierExposesImagePathConfigFields(t *testing.T) {
	meta := NewAPISupplier().Meta()
	fields := map[string]bool{}
	for _, field := range meta.ConfigFields {
		fields[field.Name] = true
	}
	for _, name := range []string{"image_generation_path", "image_edit_path"} {
		if !fields[name] {
			t.Fatalf("missing config field %q in %#v", name, meta.ConfigFields)
		}
	}
}
