import type { Context, Span, SpanOptions } from "@opentelemetry/api";
import { context as contextAPI, trace as traceAPI } from "@opentelemetry/api";

export const tracer = traceAPI.getTracer("dateilager-js-client");

/**
 * Run a function within a traced span.
 *
 * If the `{ root: true }` `SpanOption` is given, runs the function within a new root span and links the new span to the currently active span (if it exists). Uses OpenTelemetry span Links, which aren't supported by all otel backends.
 *
 * Otherwise, uses the currently active context to find a parent span.
 */
export function trace<T>(name: string, fn: (span: Span) => T): T;
export function trace<T>(name: string, options: SpanOptions, fn: (span: Span) => T): T;
export function trace<T>(name: string, options: SpanOptions, context: Context, fn: (span: Span) => T): T;
export function trace<T>(
  name: string,
  fnOrOptions: SpanOptions | ((span: Span) => T),
  fnOrContext?: Context | ((span: Span) => T),
  fn?: (span: Span) => T
): T {
  let run: (span: Span) => T;
  let options: SpanOptions;
  let context: Context;

  if (fn) {
    run = fn;
    options = fnOrOptions as SpanOptions;
    context = fnOrContext as Context;
  } else if (fnOrContext) {
    run = fnOrContext as typeof run;
    options = fnOrOptions as SpanOptions;
    context = contextAPI.active();
  } else {
    run = fnOrOptions as typeof run;
    options = {};
    context = contextAPI.active();
  }

  const currentSpan = traceAPI.getSpan(context);
  if (currentSpan && options.root) {
    options.links = (options.links ?? []).concat({
      context: currentSpan.spanContext(),
    });
  }

  if (options?.attributes) {
    // make sure all attributes are prefixed with "dl"
    for (const [key, value] of Object.entries(options.attributes).filter(([key]) => !key.startsWith("dl."))) {
      options.attributes["dl." + key] = value;
    }
  }

  return tracer.startActiveSpan(name, options, context, (span) => {
    try {
      const result = run(span);

      // too reduce overhead and keep the same return type, only instantiate a promise if a promise was returned
      if (result && "then" in result) {
        return Promise.resolve(result).finally(() => span.end());
      }

      span.end();
      return result;
    } catch (err: any) {
      span.end();
      throw err;
    }
  }) as T;
}

export interface WrapOptions<T extends (...args: any[]) => any> extends SpanOptions {
  /**
   * A function that receives the arguments of the wrapped function and returns attributes that will be added to the span.
   *
   * @example
   * const writeFile = wrap(
   *   "write-file",
   *   {
   *     record: (filepath, content) => ({
   *       // add attribute with the same name as the argument
   *       content,
   *
   *       // add attribute with different name than the argument
   *       path: filepath,
   *
   *       // add attribute with computed value
   *       filename: path.basename(filepath),
   *     }),
   *   },
   *   (filepath: string, content: string, sensitive: string) => {
   *     // notice we didn't add the `sensitive` argument as an attribute
   *   }
   * );
   */
  record?: (...args: Parameters<T>) => SpanOptions["attributes"];
}

/** Wrap a function in tracing, and return it */
export function wrap<T extends (...args: any[]) => any>(name: string, fn: T): T;
export function wrap<T extends (...args: any[]) => any>(name: string, options: WrapOptions<T>, fn: T): T;
export function wrap<T extends (...args: any[]) => any>(name: string, fnOrOptions: WrapOptions<T> | T, fn?: T): T {
  let run: T;
  let options: WrapOptions<T>;

  if (fn) {
    run = fn;
    options = fnOrOptions as WrapOptions<T>;
  } else {
    run = fnOrOptions as T;
    options = {};
  }

  return function (this: any, ...args: Parameters<T>) {
    if (options.record) {
      options.attributes = {
        ...options.attributes,
        ...options.record(...args),
      };
    }
    return trace(name, options, () => run.apply(this, args));
  } as T;
}

/** Method decorator */
export const traced = (name: string, options: WrapOptions<(...args: any[]) => any> = {}) => {
  return function (target: any, propertyKey: string, descriptor: PropertyDescriptor) {
    descriptor.value = wrap(name, options, descriptor.value);
    return descriptor;
  };
};
