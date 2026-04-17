import { helper } from './helper';
import fs from 'fs';

const config = require('./config');

class Calculator {
  constructor() {
    this.value = 0;
  }

  add(n) {
    this.value += n;
    return this;
  }

  subtract(n) {
    this.value -= n;
    return this;
  }

  reset() {
    this.reset();
    this.value = 0;
  }
}

class ScientificCalculator extends Calculator {
  pow(exp) {
    this.value = Math.pow(this.value, exp);
    return this;
  }
}

function createCalculator() {
  return new Calculator();
}

const multiply = (a, b) => a * b;

const divide = function(a, b) {
  return a / b;
};

export { Calculator, ScientificCalculator, createCalculator };
