package amp

import "testing"

func TestNormalizeRuntimeConfig(t *testing.T) {
	result, err := normalizeRuntimeConfig(RuntimeConfig{
		SystemUser:  "amp-user",
		ManagerPath: "/opt/cubecoders/ampinstmgr",
		WrapperPath: "/usr/local/bin/ampcontrol-amp",
		SudoPath:    "/usr/bin/sudo",
	})
	if err != nil || result.SystemUser != "amp-user" {
		t.Fatalf("configuração inesperada: %#v erro=%v", result, err)
	}
}

func TestNormalizeRuntimeConfigRejectsUnsafeValues(t *testing.T) {
	valid := RuntimeConfig{
		SystemUser:  "amp",
		ManagerPath: "/usr/bin/ampinstmgr",
		WrapperPath: "/usr/local/bin/ampcontrol-amp",
		SudoPath:    "/usr/bin/sudo",
	}

	invalidUser := valid
	invalidUser.SystemUser = "amp;root"
	if _, err := normalizeRuntimeConfig(invalidUser); err == nil {
		t.Fatal("era esperado erro para usuário inseguro")
	}
	invalidPath := valid
	invalidPath.ManagerPath = "ampinstmgr"
	if _, err := normalizeRuntimeConfig(invalidPath); err == nil {
		t.Fatal("era esperado erro para caminho relativo")
	}
}
