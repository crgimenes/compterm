-- init.lua
-- This file is executed when compterm starts.
-- You can use this file to load your own lua scripts.

Printf = function(s,...)
    return io.write(s:format(...))
end

-- ConfigPath
local path = ConfigPath()
Printf("Config path: %s\n", path)

-- ConfigDebug
-- local debug = ConfigDebug(false)
-- Printf("Config debug: %s\n", debug)

-- ConfigListen
-- local listen = ConfigListen("0.0.0.0:2200")
-- Printf("Config listen: %s\n", listen)

-- ConfigAPIListen
-- local APIListen = ConfigAPIListen("127.0.0.1:2201")
-- Printf("Config API listen: %s\n", APIListen)

-- ConfigCommand
-- local command = ConfigCommand(os.getenv("SHELL"))
-- Printf("Config command: %s\n", command)

-- ConfigMOTD
-- local MOTD = ConfigMOTD("Welcome to compterm!")
-- Printf("Config MOTD: %s\n", MOTD)

-- ConfigAPIKey
-- local APIKey = ConfigAPIKey("") -- some random string
-- if APIKey == "" then
--     Printf("Config API key: disabled\n")
-- end

