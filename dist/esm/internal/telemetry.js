import { trace as traceAPI } from "@opentelemetry/api";
/** @internal */
export const tracer = traceAPI.getTracer("@gadgetinc/dateilager");
/**
 * Run a function within a traced span.
 *
 * @internal
 */
export function trace(name, options, fn) {
    return tracer.startActiveSpan(name, options, (span) => {
        try {
            const result = fn(span);
            // to reduce overhead and keep the same return type, only instantiate a promise if a promise was returned
            if (result && typeof result == "object" && "then" in result) {
                return Promise.resolve(result).finally(() => span.end());
            }
            span.end();
            return result;
        }
        catch (err) {
            span.end();
            throw err;
        }
    });
}
//# sourceMappingURL=telemetry.js.map