# boiler-mate

boiler-mate acts as a simple MQTT bridge from wood pellet boilers compatible with
NBE V7/10/13 controllers, used by various manufacturers.

# Supported Boilers

Any boiler with a controller that is managed through [https://stokercloud.dk]
should be compatible as long as it has the correct firmware firmware installed.

Possibly compatible manufacturers/models include:

- NBE Blackstar
- NBE RTB
- Kedel
- Opop

# Controller Firmware

If your boiler has a V7 or V10 controller, you need the firmware version that
removes the user interface on the touch screen, but enables access on the local
network.

To upgrade, go to [https://www.nbe.dk/program-download/] and select "Activate Controller
for App Operation" and follow the steps.

# Thanks & Acknowledgement

Special thanks to [[Anders Nylund](https://github.com/motoz) for documenting the
NBE protocol.
