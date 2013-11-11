justrun
=======

justrun watches files and directories and will perform the command given it
when they change. Unlike similar tools, it will terminate the running command
and rerun it if more filesystem events occur. This makes it ideal for testing
servers. (For instance, a web server whose templates you are editing.)

When a directory is passed in a file path, justrun will watch all files in
that directory, but does not recurse into subdirectories. A nice trick you can
pull is using `find . -type d` and the `-stdin` option to include all
directories recursively. Use the `-i` (ignored file list) option wisely when
you do so.

Examples
--------

    justrun -c "go build && ./mywebserver -https=:10443" -i mywebserver . templates/

    find . -type d | justrun -c "grep foobar *.rb" -stdin

    mkfifo pipe1; justrun -c "grep foobar *.rb" -stdin < pipe1 & cat filelist1 filelist2 > pipe1

    justrun -c "grep foobar *.rb" -stdin -i .git < <(cat filelist1 filelist2)

Usage
-----

    $  justrun -h
    usage: justrun -c 'SOME BASH COMMAND' [FILEPATH]*
      -c="": command to run when files change in given directories
      -delay=750ms: the time to wait between runs of the command if many fs events occur
      -h=false: this help text
      -help=false: this help text
      -i="": comma-separated list of files to ignore
      -stdin=false: read list of files to track from stdin, not the command-line

Compared to other tools
-----------------------

justrun is perhaps best understood in terms of the other tools out
there. [inotify-tools][inotify-tools] is Linux only and doesn't handle process
lifetime of the commands it runs (though, this may be
desirable). [fswatch][fswatch] will wait until the command halts before
running it again (making it useless for running servers), and is OS X
only. [entr][entr] also waits until the command given finishes for
re-running. [devweb][devweb] assumes that the command being run is a binary
that takes the parameter `-addr`. [shotgun][shotgun] is only capable of
running Ruby Rack servers, and nothing else.

Not all of these contraints are bad things.
