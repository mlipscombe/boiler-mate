# boiler-mate

boiler-mate acts as a simple MQTT bridge from wood pellet boilers compatible with
NBE V7/10/13 controllers, used by various manufacturers.

## Supported Boilers

Any boiler with a controller that is managed through [https://stokercloud.dk]
should be compatible as long as it has the correct firmware firmware installed.

Possibly compatible manufacturers/models include:

- NBE Blackstar
- NBE RTB
- Kedel
- Opop

## Controller Firmware

If your boiler has a V7 or V10 controller, you need the firmware version that
removes the user interface on the touch screen, but enables access on the local
network.

To upgrade, go to [https://www.nbe.dk/program-download/] and select "Activate Controller
for App Operation" and follow the steps.

## How to Run

Synopsis:

```
    Usage of boiler-mate:
        -debug
            debug mode
        -host string
            controller URI, in the format tcp://<serial>:<password>@<host>:<port>
        -mqtt string
            MQTT URI, in the format tcp://[<user>:<password>]@<host>:<port>[/<prefix>]
            (default "tcp://localhost:1883")
```

Example:

```
    boiler-mate --host udp://3629:0587451614@192.168.1.100:8483 --mqtt tcp://10.10.11.20:1883
```

The boiler's password is required to write settings, but not to read them. You can
find controller's serial number and password in the top right corner of the display
on the unit.

If an MQTT prefix is not specified, messages will be published to the `nbe/<serial>`
topic.

## Thanks & Acknowledgement

Special thanks to [Anders Nylund](https://github.com/motoz) for documenting the
NBE protocol.
