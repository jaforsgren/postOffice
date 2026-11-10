// Chai-compatible BDD assertions backing pm.expect. Evaluated once per VM;
// the expression's value is the expect function.
(function () {
  "use strict";

  const LANGUAGE_CHAINS = [
    "to", "be", "been", "is", "that", "which", "and", "has", "have",
    "with", "at", "of", "same", "but", "does", "still", "also",
  ];
  const MAX_INSPECT_DEPTH = 3;

  class AssertionError extends Error {
    constructor(message) {
      super(message);
      this.name = "AssertionError";
    }
  }

  function inspect(value, depth) {
    const level = depth || 0;
    if (typeof value === "string") return `'${value}'`;
    if (value === null || value === undefined || typeof value !== "object") {
      if (typeof value === "function") return `[Function${value.name ? ": " + value.name : ""}]`;
      return String(value);
    }
    if (value instanceof RegExp || value instanceof Date) return String(value);
    if (level >= MAX_INSPECT_DEPTH) return Array.isArray(value) ? "[Array]" : "[Object]";
    if (Array.isArray(value)) {
      return value.length ? `[ ${value.map(v => inspect(v, level + 1)).join(", ")} ]` : "[]";
    }
    const entries = Object.keys(value).map(k => `${k}: ${inspect(value[k], level + 1)}`);
    return entries.length ? `{ ${entries.join(", ")} }` : "{}";
  }

  function typeOf(value) {
    if (value === null) return "null";
    if (Array.isArray(value)) return "array";
    if (value instanceof RegExp) return "regexp";
    if (value instanceof Date) return "date";
    return typeof value;
  }

  function deepEqual(a, b) {
    if (a === b) return true;
    if (typeof a === "number" && typeof b === "number") return Number.isNaN(a) && Number.isNaN(b);
    if (typeOf(a) !== typeOf(b) || typeof a !== "object" || a === null) return false;
    if (a instanceof Date) return a.getTime() === b.getTime();
    if (a instanceof RegExp) return String(a) === String(b);
    if (Array.isArray(a)) {
      return a.length === b.length && a.every((v, i) => deepEqual(v, b[i]));
    }
    const keysA = Object.keys(a);
    const keysB = Object.keys(b);
    return keysA.length === keysB.length && keysA.every(k => Object.prototype.hasOwnProperty.call(b, k) && deepEqual(a[k], b[k]));
  }

  function sizeOf(value) {
    if (value instanceof Map || value instanceof Set) return value.size;
    return value.length;
  }

  function Assertion(subject, message) {
    this._subject = subject;
    this._message = message;
    this._flags = {};
  }

  Assertion.prototype.assert = function (passed, positive, negative, expected, actual) {
    const negate = this._flags.negate === true;
    if (negate ? !passed : passed) return;
    let text = negate ? negative : positive;
    text = text
      .replace(/#\{this\}/g, () => inspect(this._subject))
      .replace(/#\{exp\}/g, () => inspect(expected))
      .replace(/#\{act\}/g, () => inspect(actual === undefined ? this._subject : actual));
    throw new AssertionError(this._message ? `${this._message}: ${text}` : text);
  };

  function addChain(name) {
    Object.defineProperty(Assertion.prototype, name, { get() { return this; }, configurable: true });
  }

  function addFlag(name, flag) {
    Object.defineProperty(Assertion.prototype, name, {
      get() { this._flags[flag] = true; return this; },
      configurable: true,
    });
  }

  function addProperty(name, check) {
    Object.defineProperty(Assertion.prototype, name, {
      get() { check.call(this); return this; },
      configurable: true,
    });
  }

  function addMethod(names, fn) {
    for (const name of [].concat(names)) {
      Assertion.prototype[name] = function (...args) { fn.apply(this, args); return this; };
    }
  }

  // Chainable methods work both as a call (`.include(x)`) and as a flag that
  // later assertions read (`.include.keys(...)`, `.length.above(2)`).
  function addChainableMethod(names, fn, flag) {
    for (const name of [].concat(names)) {
      Object.defineProperty(Assertion.prototype, name, {
        get() {
          const assertion = this;
          if (flag) assertion._flags[flag] = true;
          const callable = function (...args) { fn.apply(assertion, args); return assertion; };
          Object.setPrototypeOf(callable, assertion);
          return callable;
        },
        configurable: true,
      });
    }
  }

  LANGUAGE_CHAINS.forEach(addChain);
  addFlag("not", "negate");
  addFlag("deep", "deep");
  addFlag("any", "any");
  addFlag("all", "all");
  addFlag("own", "own");
  addFlag("nested", "nested");

  addProperty("ok", function () {
    this.assert(!!this._subject, "expected #{this} to be truthy", "expected #{this} to be falsy");
  });
  addProperty("true", function () {
    this.assert(this._subject === true, "expected #{this} to be true", "expected #{this} to be false");
  });
  addProperty("false", function () {
    this.assert(this._subject === false, "expected #{this} to be false", "expected #{this} to be true");
  });
  addProperty("null", function () {
    this.assert(this._subject === null, "expected #{this} to be null", "expected #{this} not to be null");
  });
  addProperty("undefined", function () {
    this.assert(this._subject === undefined, "expected #{this} to be undefined", "expected #{this} not to be undefined");
  });
  addProperty("NaN", function () {
    this.assert(Number.isNaN(this._subject), "expected #{this} to be NaN", "expected #{this} not to be NaN");
  });
  addProperty("exist", function () {
    this.assert(this._subject !== null && this._subject !== undefined,
      "expected #{this} to exist", "expected #{this} to not exist");
  });
  addProperty("empty", function () {
    const subject = this._subject;
    let isEmpty;
    if (typeof subject === "string" || Array.isArray(subject)) isEmpty = subject.length === 0;
    else if (subject instanceof Map || subject instanceof Set) isEmpty = subject.size === 0;
    else if (subject !== null && typeof subject === "object") isEmpty = Object.keys(subject).length === 0;
    else throw new TypeError(`.empty was passed non-string primitive ${inspect(subject)}`);
    this.assert(isEmpty, "expected #{this} to be empty", "expected #{this} not to be empty");
  });

  addChainableMethod(["a", "an"], function (type) {
    const expected = String(type).toLowerCase();
    const article = /^[aeiou]/.test(expected) ? "an" : "a";
    this.assert(typeOf(this._subject) === expected,
      `expected #{this} to be ${article} ${expected}`, `expected #{this} not to be ${article} ${expected}`);
  }, null);

  addChainableMethod(["include", "includes", "contain", "contains"], function (value) {
    const subject = this._subject;
    let passed;
    if (typeof subject === "string") {
      passed = subject.includes(value);
    } else if (Array.isArray(subject)) {
      passed = this._flags.deep ? subject.some(item => deepEqual(item, value)) : subject.includes(value);
    } else if (subject instanceof Set || subject instanceof Map) {
      passed = subject.has(value);
    } else if (subject !== null && typeof subject === "object" && value !== null && typeof value === "object") {
      const compare = this._flags.deep ? deepEqual : (a, b) => a === b;
      passed = Object.keys(value).every(k => k in subject && compare(subject[k], value[k]));
    } else {
      throw new TypeError(`the given combination of arguments (${typeOf(subject)} and ${typeOf(value)}) is invalid for this assertion`);
    }
    this.assert(passed, "expected #{this} to include #{exp}", "expected #{this} to not include #{exp}", value);
  }, "contains");

  addChainableMethod(["length", "lengthOf"], function (expected) {
    const actual = sizeOf(this._subject);
    this.assert(actual === expected,
      `expected #{this} to have a length of ${expected} but got ${actual}`,
      `expected #{this} to not have a length of ${expected}`, expected, actual);
  }, "doLength");

  function compareTo(name, compare) {
    return function (bound) {
      const measured = this._flags.doLength ? sizeOf(this._subject) : this._subject;
      if (!this._flags.doLength && typeof measured !== "number" && !(measured instanceof Date)) {
        throw new TypeError(`expected ${inspect(measured)} to be a number or a date`);
      }
      const body = this._flags.doLength ? `have a length ${name} ${bound}` : `be ${name} ${inspect(bound)}`;
      this.assert(compare(measured, bound), `expected #{this} to ${body}`, `expected #{this} to not ${body}`);
    };
  }
  addMethod(["above", "gt", "greaterThan"], compareTo("above", (a, b) => a > b));
  addMethod(["least", "gte", "greaterThanOrEqual"], compareTo("at least", (a, b) => a >= b));
  addMethod(["below", "lt", "lessThan"], compareTo("below", (a, b) => a < b));
  addMethod(["most", "lte", "lessThanOrEqual"], compareTo("at most", (a, b) => a <= b));

  addMethod("within", function (start, finish) {
    const measured = this._flags.doLength ? sizeOf(this._subject) : this._subject;
    this.assert(measured >= start && measured <= finish,
      `expected #{this} to be within ${start}..${finish}`, `expected #{this} to not be within ${start}..${finish}`);
  });

  addMethod(["equal", "equals", "eq"], function (expected) {
    if (this._flags.deep) {
      this.assert(deepEqual(this._subject, expected),
        "expected #{this} to deeply equal #{exp}", "expected #{this} to not deeply equal #{exp}", expected);
      return;
    }
    this.assert(this._subject === expected,
      "expected #{this} to equal #{exp}", "expected #{this} to not equal #{exp}", expected);
  });

  addMethod(["eql", "eqls"], function (expected) {
    this.assert(deepEqual(this._subject, expected),
      "expected #{this} to deeply equal #{exp}", "expected #{this} to not deeply equal #{exp}", expected);
  });

  addMethod("oneOf", function (list) {
    const passed = this._flags.deep ? list.some(item => deepEqual(item, this._subject)) : list.includes(this._subject);
    this.assert(passed, "expected #{this} to be one of #{exp}", "expected #{this} to not be one of #{exp}", list);
  });

  addMethod("match", function (pattern) {
    this.assert(pattern.test(this._subject),
      `expected #{this} to match ${pattern}`, `expected #{this} not to match ${pattern}`);
  });

  addMethod("string", function (fragment) {
    this.assert(typeof this._subject === "string" && this._subject.includes(fragment),
      "expected #{this} to contain #{exp}", "expected #{this} to not contain #{exp}", fragment);
  });

  addMethod("instanceof", function (constructor) {
    this.assert(this._subject instanceof constructor,
      `expected #{this} to be an instance of ${constructor.name}`,
      `expected #{this} to not be an instance of ${constructor.name}`);
  });

  addMethod("satisfy", function (matcher) {
    this.assert(matcher(this._subject), "expected #{this} to satisfy matcher", "expected #{this} to not satisfy matcher");
  });

  // .property(name[, value]) moves the subject to that property, as in Chai,
  // so `.to.have.property("id").that.is.a("string")` asserts on the id.
  Assertion.prototype.property = function (name, ...value) {
    const subject = this._subject;
    const has = subject !== null && subject !== undefined &&
      (this._flags.own ? Object.prototype.hasOwnProperty.call(subject, name) : name in Object(subject));
    const actual = has ? subject[name] : undefined;

    if (value.length === 0) {
      this.assert(has, `expected #{this} to have property '${name}'`, `expected #{this} to not have property '${name}'`);
    } else {
      const matches = has && (this._flags.deep ? deepEqual(actual, value[0]) : actual === value[0]);
      this.assert(matches,
        `expected #{this} to have property '${name}' of #{exp}, but got #{act}`,
        `expected #{this} to not have property '${name}' of #{exp}`, value[0], actual);
    }
    if (!this._flags.negate) this._subject = actual;
    return this;
  };
  Assertion.prototype.ownProperty = function (name, ...value) {
    this._flags.own = true;
    return this.property(name, ...value);
  };

  Assertion.prototype.keys = function (...args) {
    const expected = args.length === 1 && typeof args[0] === "object" && args[0] !== null
      ? (Array.isArray(args[0]) ? args[0] : Object.keys(args[0]))
      : args;
    const actual = this._subject instanceof Map || this._subject instanceof Set
      ? Array.from(this._subject.keys())
      : Object.keys(this._subject);
    const present = expected.filter(k => actual.includes(k));
    let passed;
    if (this._flags.any) passed = present.length > 0;
    else if (this._flags.contains) passed = present.length === expected.length;
    else passed = present.length === expected.length && actual.length === expected.length;
    const quantifier = this._flags.any ? "any of" : "all of";
    this.assert(passed,
      `expected #{this} to have ${quantifier} keys #{exp}`, `expected #{this} to not have ${quantifier} keys #{exp}`, expected);
    return this;
  };
  Assertion.prototype.key = Assertion.prototype.keys;

  Assertion.prototype.members = function (expected) {
    const subject = this._subject;
    const has = (list, item) => this._flags.deep ? list.some(v => deepEqual(v, item)) : list.includes(item);
    const passed = this._flags.contains
      ? expected.every(item => has(subject, item))
      : subject.length === expected.length && expected.every(item => has(subject, item)) && subject.every(item => has(expected, item));
    const label = this._flags.contains ? "be a superset of" : "have the same members as";
    this.assert(passed, `expected #{this} to ${label} #{exp}`, `expected #{this} to not ${label} #{exp}`, expected);
    return this;
  };

  addMethod("throw", function (expected) {
    let thrown = null;
    try { this._subject(); } catch (err) { thrown = err; }
    let passed = thrown !== null;
    if (passed && expected instanceof RegExp) passed = expected.test(String(thrown && thrown.message));
    else if (passed && typeof expected === "string") passed = String(thrown && thrown.message).includes(expected);
    this.assert(passed, "expected #{this} to throw", "expected #{this} to not throw");
  });

  function expect(subject, message) {
    return new Assertion(subject, message);
  }
  expect.fail = function (actualOrMessage, expected, message) {
    const text = arguments.length < 2 ? actualOrMessage : (message || `expected ${inspect(actualOrMessage)} to equal ${inspect(expected)}`);
    throw new AssertionError(text === undefined ? "expect.fail()" : text);
  };
  expect.AssertionError = AssertionError;

  return expect;
})()
