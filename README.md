justrun
=======

Justrun watches files and directories and will perform the command given it
when those files change. Unlike similar tools, it will terminate the running
command and rerun it if more filesystem events occur. This makes it ideal for
testing servers. (For instance, a web server whose templates you are editing.)

Justrun also lets you say how long to wait before running the command again,
even if filesystem events have occurred in the meantime. See the `-delay`
option in the [Usage section][usage].

When a directory is passed in as an argument, justrun will watch all
files in that directory, but does not recurse into subdirectories. If
you need that recursion, a trick you can pull is using `find . -type d`
and the `-stdin` option to include all directories
recursively. When playing tricks like this, use the ignored file list
option (`-i`) wisely. If not, you'll accidentally watch files that
your command touch, and put your commands into an infinite loop.

Justrun does kill the child processes of the bash command run by it to
end the lifecycles of long-lived (that is, server) processes. If want
justrun to wait for the commands to finish before checking for more
filesystem changes, add the `-w` argument to the commandline.

Examples
--------

    justrun -c 'go build && ./mywebserver -https=:10443' -i mywebserver . templates/

    justrun -c 'make' -w -i mylib.a -i mylib.so .

    find . -type d | justrun -c 'grep foobar *.h' -stdin -i .git

    justrun -c 'grep foobar *.h' -stdin < <(cat filelist1 filelist2)

    justrun -c 'some_expensive_op' -delay 10s .

    justrun -c 'some_inexpensive_op' -delay 100ms .

Usage
-----

    $  justrun -h
    justrun: help requested
    usage: justrun -c 'SOME BASH COMMAND' [FILEPATH]*
      -c="": command to run when files change in given directories
      -delay=750ms: the time to wait between runs of the command if many fs events occur
      -h=false: print this help text
      -help=false: print this help text
      -i=[]: a file path to ignore events from (may be given multiple times)
      -stdin=false: read list of files to track from stdin, not the command-line
      -v=false: verbose output
      -w=false: wait for the command to finish and do not attempt to kill it
      -s=bash: shell to run the command


Compared to other tools
-----------------------

Justrun is perhaps best understood in terms of the other tools out
there. [inotify-tools][inotify-tools] is Linux only and doesn't handle process
lifetime of the commands it runs (though, this may be desirable) so its
difficult to make servers run well with it. [fswatch][fswatch] similarly will
wait until the command halts before running it again, and is OS X
only. [entr][entr] also waits until the command given finishes for
re-running. [devweb][devweb] assumes that the command being run is a binary
that takes the parameter `-addr`. [shotgun][shotgun] is only capable of
running Ruby Rack servers, and nothing else. [nailgun][nailgun] requires the
commands be written in Java, and are run in the nailgun server's process space
instead of the user's shell.

Not all of the constraints on these other tools are bad choices.

Installing
----------

The easiest way to install justrun is to put the justrun binary in [the published
zipfiles][download] into your PATH. That's it! You can find all of the
pre-built binaries at [http://projects.somethingsimilar.com/justrun/downloads/][download]

To install from source, [install Go][installgo] (being sure to set up a working
`$GOPATH`, detailed in those instructions), and run:

    go get github.com/jmhodges/justrun

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

The `-i` argument is no longer required to be a comma-separated list,
but it would be nice for more complicated systems to have an easier
way to configure ignore lists.

It's fairly easy to accidentally cause a cycle in your commands and the
filesystem watches. Files or directories that will be touched or created by
your command should be added to the `-i` option.

If you wish to fork off subprocessses in your commands, you'll have to call
[`setpgid(2)`][setpgid] (or `set -o monitor` in the bash shell) in the
commands to avoid having them terminated.

[usage]: https://github.com/jmhodges/justrun#usage
[setpgid]: http://linux.die.net/man/2/setpgid
[inotify-tools]: https://github.com/rvoicilas/inotify-tools/wiki
[fswatch]: https://github.com/alandipert/fswatch
[entr]: http://entrproject.org/
[devweb]: https://code.google.com/p/rsc/source/browse/devweb
[shotgun]: https://github.com/rtomayko/shotgun
[nailgun]: http://www.martiansoftware.com/nailgun/
[installgo]: http://golang.org/doc/install#install
[download]: http://projects.somethingsimilar.com/justrun/downloads
