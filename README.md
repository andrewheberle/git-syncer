# git-syncer

This is very similar to `git-sync` but much more narrow in scope and less
generic as far as:

1. HTTP credentials for Git can be retrieved from a Consul KV store
2. A reload command can be specified
3. The reload command will only be run if the filter (a regexp) matches the
	changed files

This is very early days so there are likely a lot of bugs here.
