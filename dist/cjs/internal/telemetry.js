"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.trace = exports.tracer = void 0;
const api_1 = require("@opentelemetry/api");
/** @internal */
exports.tracer = api_1.trace.getTracer("@gadgetinc/dateilager");
/**
 * Run a function within a traced span.
 *
 * @internal
 */
function trace(name, options, fn) {
    return exports.tracer.startActiveSpan(name, options, (span) => {
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
exports.trace = trace;
//# sourceMappingURL=telemetry.js.map