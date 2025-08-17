# Machine Data Collection Proxy

![MIT License Badge](https://img.shields.io/badge/License-MIT-blue)

`mdcmux` provides a proxy server for Haas Automation Next Generation Controls to
improve upon the existing [Machine Data
Collection](https://www.haascnc.com/service/troubleshooting-and-how-to/how-to/machine-data-collection---ngc.html#gsc.tab=0)
network protocol. This ultimately allows machine data to be more readily
available outside the locked-down LAN to which the machine controls are
attached. This supports a number of dashboard and software integrations I'm
building over at [CNC.LLC](https://cnc.llc).

Features:

* [X] Allow an arbitrary number of network clients to connect to an arbitrary number of controls.
* [X] Parse, validate, and proxy individual MDC messages.
* [X] Restrict access and allowable MDC commands by IP address range.
* [ ] Allow / deny writes to certain macro variable ranges (e.g. WIPS calibration data).
* [ ] Audit logging

MDC protocol enhancements:

* [ ] Selecting backends on the fly.
* [ ] Support for TLS between clients and the proxy.
* [ ] Support JWT-based policy decisions.

## Rationale

The Machine Data Collection protocol allows network clients to interact with an
NGC control, allowing them to retrieve information about the machine and to PEEK
and POKE macro variables at runtime. The current implementation of MDC allows
two simultaneous network connections, but replies to queries from any connection
are broadcast to all connections. The protocol relies on network isolation for
security and does not provide a way to restrict the actions permitted by a
network actor. These features make it somewhat awkward to build multiple,
independent, integrations with NGC controls. `mdcmux` ensures that multiple
clients cannot accidentally interfere with one another and allows only certain
actors to execute commands which could negatively impact the MDC host.

## Installing

GitHub workflows are currently TODO.

`go install vawter.tech/mdcmux@main`

## Proxy use

`mdcmux start -c config.json -v`

The configuration file defines an IP address for the proxy to bind to, which
defaults to `localhost`. Multiple MDC targets may be defined and are proxied on
separate ports.

Security policies are currently defined on a netblock basis. By default,
`mdcmux` prevents use of the `?E` command and any `?Q` command number not listed
in the MDC documentation. Policies can be defined at the top level of the
configuration file or on a per-target basis.

```json
{
  "bind": "127.0.0.1",
  "policy": {
    "127.0.0.1/32": {
      "allow_unsafe": false
    }
  },
  "targets": {
    "minimill.cnc.llc:5501": {
      "proxy_port": 5051
    },
    "umc750.cnc.llc:5501": {
      "proxy_port": 5052,
      "policy": {
        "10.1.0.0/16": {
          "allow_unsafe": true
        }
      }
    }
  }
}
```

The above configuration would proxy the MDC service on two different NGC controls to ports `5051` and `5052`.

## Dummy server

The `mdcmux` binary contains a trivial MDC server implementation, with canned
replied to most `Q` codes. It does support `?Q600` and `?E` commands.

```
mdcmux dummy --bind 127.0.0.1:13013 &

# You can use PuTTY, etc.
nc 127.0.0.1 13013
?Q102
>>MODEL, MDCMUX
?Q101
>>SOFTWARE VERSION, 100.24.000.1024
?Q100
>>SERIAL NUMBER, 1024
?Q600 10900
>>MACRO, 0.0
?E10900 123.456
>>!
?Q600 10900
>>MACRO, 123.456
```

## Disclaimer

This software is provided as-is, without warranty of any kind.

This project is not associated with or endorsed by Haas Automation, Inc.