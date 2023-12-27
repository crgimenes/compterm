-- init.lua
-- This file is executed when compterm starts.
-- You can use this file to load your own lua scripts.

printf = function(s,...)
    return io.write(s:format(...))
end

configPath = ConfigPath()
printf("Config path: %s\n", configPath)

