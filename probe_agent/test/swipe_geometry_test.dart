import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';

void main() {
  test('no distance is the default, 50 percent', () {
    expect(swipeFraction(0), 0.5);
  });

  test('a distance is that percent of the element or the screen', () {
    expect(swipeFraction(40), 0.4);
    expect(swipeFraction(1), 0.01);
    expect(swipeFraction(100), 1.0);
  });

  test('a distance above 100 is clamped to 100, and a negative one is the default', () {
    expect(swipeFraction(150), 1.0);
    expect(swipeFraction(-5), 0.5);
  });

  test('the delta is the size times the fraction, in the direction of the swipe', () {
    const size = Size(400, 800);
    expect(swipeDelta('up', size, 0.5), const Offset(0, -400));
    expect(swipeDelta('down', size, 0.5), const Offset(0, 400));
    expect(swipeDelta('left', size, 0.5), const Offset(-200, 0));
    expect(swipeDelta('right', size, 0.5), const Offset(200, 0));
  });

  test('a distance is applied once: 40 percent of the height is 320, not 40 percent of 40 percent', () {
    const size = Size(400, 800);
    expect(swipeDelta('up', size, swipeFraction(40)), const Offset(0, -320));
  });

  test('100 percent ends one half-size past the edge: from the centre at 400, the finger ends at -400', () {
    const size = Size(400, 800);
    expect(swipeDelta('up', size, swipeFraction(100)), const Offset(0, -800));
  });
}
