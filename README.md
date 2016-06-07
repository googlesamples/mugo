# µgo

[![Join the chat at https://gitter.im/googlesamples/mugo](https://badges.gitter.im/googlesamples/mugo.svg)](https://gitter.im/googlesamples/mugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

Sample on how to transpile a small subset of go to Arduino sketches using [go/ast](https://golang.org/pkg/go/ast/).

```
🍡  µ < blink/blink.go
void setup() {
  pinMode(13, OUTPUT);
}
void loop() {
  digitalWrite(13, HIGH);
  delay(1000);
  digitalWrite(13, LOW);
  delay(1000);
}
```

# Disclaimer

This is not an official Google product.
