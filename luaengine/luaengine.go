package luaengine

import (
	"sync"

	"github.com/crgimenes/compterm/config"
	lua "github.com/yuin/gopher-lua"
)

var (
	mx sync.Mutex
)

func ConfigDebug(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.Debug = L.ToBool(1)
	}
	L.Push(lua.LBool(config.CFG.Debug))
	return 1
}

func ConfigListen(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.Listen = L.ToString(1)
	}
	L.Push(lua.LString(config.CFG.Listen))
	return 1
}

func ConfigAPIListen(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.APIListen = L.ToString(1)
	}
	L.Push(lua.LString(config.CFG.APIListen))
	return 1
}

func ConfigCommand(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.Command = L.ToString(1)
	}
	L.Push(lua.LString(config.CFG.Command))
	return 1
}

func ConfigMOTD(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.MOTD = L.ToString(1)
	}
	L.Push(lua.LString(config.CFG.MOTD))
	return 1
}

func ConfigAPIKey(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.APIKey = L.ToString(1)
	}
	L.Push(lua.LString(config.CFG.APIKey))
	return 1
}

func ConfigPath(L *lua.LState) int {
	mx.Lock()
	defer mx.Unlock()
	if L.GetTop() == 1 {
		config.CFG.Path = L.ToString(1)
	}
	L.Push(lua.LString(config.CFG.Path))
	return 1
}

func Startup(initLua string) error {
	L := lua.NewState()
	defer L.Close()
	L.SetGlobal("ConfigPath", L.NewFunction(ConfigPath))
	L.SetGlobal("ConfigDebug", L.NewFunction(ConfigDebug))
	L.SetGlobal("ConfigListen", L.NewFunction(ConfigListen))
	L.SetGlobal("ConfigAPIListen", L.NewFunction(ConfigAPIListen))
	L.SetGlobal("ConfigCommand", L.NewFunction(ConfigCommand))
	L.SetGlobal("ConfigMOTD", L.NewFunction(ConfigMOTD))
	L.SetGlobal("ConfigAPIKey", L.NewFunction(ConfigAPIKey))
	return L.DoFile(initLua)
}
