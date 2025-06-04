# Jerry-Rigged Korean BBQ Robot (JRKBR)

## But why?

My wife and I love our [local Korean BBQ / Hot Pot joint](https://www.meetshotpot.com).

Tasty food and you don't even have to do the dishes! Can't go wrong!

Anywho, we have both become enamored by
their [Purdu Robotics Bellabot](https://www.pudurobotics.com/product/detail/bellabot) that occasionally brings us food
in place of a human with a cart. My wife joked, "How cool would it be to own one of these?"

As I scarfed down my sixth portion of spicy bulgogi, the hamster wheel in my head started spinning. You'd notice the
smoke coming from my ears if not for the burning gochujang fusing with the grill.

I remember reading about iRobot selling a [dev kit](https://edu.irobot.com/what-we-offer/create3). It appears it is sold
out everywhere; not that it matters I wasn't going to spend $300 dollars on this funny thought.

Then it dawned on me, I have an old Roomba 880 in the basement collecting dust. I wondered if you can control it like
the Create 3 dev kit.

TLDR; you can by using the [Serial Command Interface](resources/Roomba_SCI_Specification.pdf) via serial port using
hidden under the handle!

Whelp, here goes nothing!

---

## Bill of Materials

- Roomba 880
- [USB UART TTL Serial Port Cable for Roomba](https://www.amazon.com/dp/B0838TGLTW)
    - If (when) this link goes dead, [here is a screenshot](resources/images/serial_cable.png)
- [7 Inch IPS LCD Capacitive Touch Screen, 1024x768](https://www.amazon.com/dp/B09XKC53NH)
    - Again link rot dictates this will go dead, [here is a screenshot](resources/images/screen.png)
    - I wasn't going to have a screen but I need to replicate the cute cat found on the Bellabot
- Raspberry Pi 4 Model B
- Logitech Brio 4K Webcam (Overkill, but I owned it)
- Large Capacity Battery Bank
- Steel flat bars with holes x2
- Chafing dishes that can hold two half pans x2
- Custom 3D printed parts
    - GoPro style mount for the webcam
    - Brackets to attach flat bar rails to the Roomba
    - Brackets to attach chafing dishes to flat bar
    - Brackets to attach the screen to the flat bar
- Assorted USB Cables to attach everything
- Painter's Tape

---

## The Gameplan

- [x] Control the Roomba via SPI with Web Interface
- [x] Color Detection / Color Centering Proof of Concept
- [ ] Find the specified color (painters' tape) and center the Roomba nose on it
- [ ] Follow the line until the line cannot be seen anymore

With this setup the Roomba will rotate until the painter tape is in view, center the tape in the frame, follow the tape
and then stop when it reaches the end of the tape.

