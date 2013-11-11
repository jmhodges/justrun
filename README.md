justrun
=======

Justrun watches files and directories and will perform the command given it
when they change. Unlike similar tools, it will terminate the running command
and rerun it if more filesystem events occur. This makes it ideal for testing
servers. (For instance, a web server whose templates you are editing.)

When a directory is passed in a file path, justrun will watch all files in
that directory, but does not recurse into subdirectories. A nice trick you can
pull is using `find . -type d` and the `-stdin` option to include all
directories recursively. When playing tricks like this, use the ignored file
list option (`-i`) wisely. If not, you'll accidentally watch files that your
command touch, and put your commands into an infinite loop.

Justrun also lets you say how long to wait before running the command again,
even if filesystem events have occurred in the meantime. See the `-delay`
option in the [Usage section][usage].

Justrun does kill the child processes of commands run in it in order to handle
the lifecycles of long-lived (that is, server) processes. If you do not want
justrun to wait for the commands to finish instead, add the `-w` argument to
the commandline. If you wish to fork off subprocessses in your commands,
you'll have to call [`setpgid(2)`][setpgid] (or `set -o monitor` in the bash
shell) in the commands to avoid having them terminated.

Examples
--------

    justrun -c 'go build && ./mywebserver -https=:10443' -i mywebserver . templates/

    justrun -c 'make' -w -i mylib.a,.git .

    find . -type d | justrun -c 'grep foobar *.h' -stdin -i .git

    justrun -c 'grep foobar *.h' -stdin < <(cat filelist1 filelist2)

    justrun -c 'some_expensive_op' -delay 10s .

    justrun -c 'some_inexpensive_op' -delay 100ms .

Usage
-----

    $  justrun -h
    usage: justrun -c 'SOME BASH COMMAND' [FILEPATH]*
      -c="": command to run when files change in given directories
      -delay=750ms: the time to wait between runs of the command if many fs events occur
      -h=false: print this help text
      -help=false: print this help text
      -i="": comma-separated list of files to ignore
      -stdin=false: read list of files to track from stdin, not the command-line
      -w=false: wait for the command to finish and do not attempt to kill it

Compared to other tools
-----------------------

Justrun is perhaps best understood in terms of the other tools out
there. [inotify-tools][inotify-tools] is Linux only and doesn't handle process
lifetime of the commands it runs (though, this may be
desirable). [fswatch][fswatch] will wait until the command halts before
running it again (making it useless for running servers), and is OS X
only. [entr][entr] also waits until the command given finishes for
re-running. [devweb][devweb] assumes that the command being run is a binary
that takes the parameter `-addr`. [shotgun][shotgun] is only capable of
running Ruby Rack servers, and nothing else. [nailgun][nailgun] requires the
commands be written in Java, and are run in the nailgun server's process space
instead of the user's shell.

Not all of the contraints on these other tools are bad choices.

Caveats
-------

Justrun requires commands to handle SIGTERM as their termination signal (or
one of their termination signals). It does not attempt to send SIGKILL if the
processes do not shutdown "quickly" in response to a SIGTERM.

Justrun runs on *nixes only.

Justrun will always send a SIGTERM to its child processes, even if it received
a SIGINT.

Justrun currently only supports the bash shell, but, with some thought, a
shell configuration option could be provided. Pull requests welcome.

The `-i` argument being a comma-separated list is gross. I've not found a
better mechanism, yet. Globbing in it may help. Pull requests welcome.

It's fairly easy to accidentally cause a cycle in your commands and the
filesystem watches. Add files or directories touched or created by your
commands to the `-i` option.

[usage]: https://github.com/jmhodges/justrun#usage
[setpgid]: http://linux.die.net/man/2/setpgid
[inotify-tools]: https://github.com/rvoicilas/inotify-tools/wiki
[fswatch]: https://github.com/alandipert/fswatch
[entr]: http://entrproject.org/
[devweb]: https://code.google.com/p/rsc/source/browse/devweb
[shotgun]: https://github.com/rtomayko/shotgun
[nailgun]: http://www.martiansoftware.com/nailgun/
