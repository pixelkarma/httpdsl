# Builtins Reference

- Builtins are listed by callable surface (functions, namespaces, and method-like APIs).
- Availability can depend on runtime context (session enabled, SSE route, DB driver configured).

## Core Functions

Kind: `functions`

### `print`

- Syntax: `print(...values)`
- Summary: Prints values to stdout/log output.

### `log`

- Syntax: `log(...values)`
- Summary: Structured log message.

### `log_info`

- Syntax: `log_info(...values)`
- Summary: Info-level log.

### `log_warn`

- Syntax: `log_warn(...values)`
- Summary: Warn-level log.

### `log_error`

- Syntax: `log_error(...values)`
- Summary: Error-level log.

### `env`

- Syntax: `env(name)`
- Summary: Reads environment variable.

### `sleep`

- Syntax: `sleep(ms)`
- Summary: Sleeps current flow for milliseconds.

### `fetch`

- Syntax: `fetch(url[, options])`
- Summary: HTTP client request.

### `exec`

- Syntax: `exec(command[, options])`
- Summary: Executes shell command.

### `now`

- Syntax: `now()`
- Summary: Unix timestamp seconds.

### `now_ms`

- Syntax: `now_ms()`
- Summary: Unix timestamp milliseconds.

### `server_stats`

- Syntax: `server_stats()`
- Summary: Returns runtime server metrics.

### `await`

- Syntax: `await(task)`
- Summary: Waits for async task result.

### `race`

- Syntax: `race(tasks)`
- Summary: Resolves first task completion.

### `render`

- Syntax: `render(template[, data])`
- Summary: Renders configured template.

### `redirect`

- Syntax: `redirect(url[, status])`
- Summary: Returns redirect response payload.

### `broadcast`

- Syntax: `broadcast(data)`
- Summary: Broadcasts to SSE clients.
- Availability: Only meaningful when SSE is enabled.

### `set_session_store`

- Syntax: `set_session_store(adapter)`
- Summary: Sets backing session store.

### `csrf_token`

- Syntax: `csrf_token()`
- Summary: Gets CSRF token for active session.
- Availability: Requires session+CSRF configuration.

### `csrf_field`

- Syntax: `csrf_field()`
- Summary: Gets CSRF hidden form field markup.
- Availability: Requires session+CSRF configuration.

## Types and Conversion

Kind: `functions`

### `len`

- Syntax: `len(value)`
- Summary: Length of string/array/map.

### `type`

- Syntax: `type(value)`
- Summary: Runtime type name.

### `str`

- Syntax: `str(value)`
- Summary: Converts to string.

### `int`

- Syntax: `int(value)`
- Summary: Converts to integer.

### `float`

- Syntax: `float(value)`
- Summary: Converts to float.

### `bool`

- Syntax: `bool(value)`
- Summary: Converts to boolean.

## String Functions

Kind: `functions`

### `trim`

- Syntax: `trim(text)`
- Summary: Trims outer whitespace.

### `split`

- Syntax: `split(text, sep)`
- Summary: Splits string to array.

### `join`

- Syntax: `join(arr, sep)`
- Summary: Joins array into string.

### `upper`

- Syntax: `upper(text)`
- Summary: Uppercases string.

### `lower`

- Syntax: `lower(text)`
- Summary: Lowercases string.

### `replace`

- Syntax: `replace(text, old, new)`
- Summary: Replaces substring.

### `starts_with`

- Syntax: `starts_with(text, prefix)`
- Summary: Prefix check.

### `ends_with`

- Syntax: `ends_with(text, suffix)`
- Summary: Suffix check.

### `contains`

- Syntax: `contains(container, needle)`
- Summary: Contains check for string/array.

### `index_of`

- Syntax: `index_of(container, needle)`
- Summary: First index, or -1.

### `repeat`

- Syntax: `repeat(text, n)`
- Summary: String repeat.

### `slice`

- Syntax: `slice(value, start[, end])`
- Summary: Slice string or array.

### `pad_left`

- Syntax: `pad_left(text, len[, pad])`
- Summary: Left pad.

### `pad_right`

- Syntax: `pad_right(text, len[, pad])`
- Summary: Right pad.

### `truncate`

- Syntax: `truncate(text, maxLen)`
- Summary: Truncates with ellipsis semantics.

### `capitalize`

- Syntax: `capitalize(text)`
- Summary: Capitalizes first letter.

### `regex_match`

- Syntax: `regex_match(pattern, text)`
- Summary: Regex match.

### `regex_replace`

- Syntax: `regex_replace(pattern, repl, text)`
- Summary: Regex replace.

## Array Functions

Kind: `functions`

### `append`

- Syntax: `append(arr, value[, ...])`
- Summary: Returns new array with appended values.

### `push`

- Syntax: `push(arr, value[, ...])`
- Summary: Alias behavior of append in expression form.

### `reverse`

- Syntax: `reverse(arr)`
- Summary: Reversed copy.

### `unique`

- Syntax: `unique(arr)`
- Summary: Deduplicated array.

### `flat`

- Syntax: `flat(arr[, depth])`
- Summary: Flattens nested arrays.

### `sort`

- Syntax: `sort(arr)`
- Summary: Sorted copy.

### `sort_by`

- Syntax: `sort_by(arr, keyOrFn)`
- Summary: Sort by key or selector.

### `chunk`

- Syntax: `chunk(arr, size)`
- Summary: Chunks array into sub-arrays.

### `range`

- Syntax: `range(end) | range(start, end[, step])`
- Summary: Generates integer range array.

## Object Functions

Kind: `functions`

### `keys`

- Syntax: `keys(obj)`
- Summary: Object keys array.

### `values`

- Syntax: `values(obj)`
- Summary: Object values array.

### `merge`

- Syntax: `merge(a, b[, ...])`
- Summary: Shallow object merge.

### `patch`

- Syntax: `patch(target, patchObj)`
- Summary: Deep-ish patch merge semantics.

### `delete`

- Syntax: `delete(obj, key)`
- Summary: Returns object without key.

## Functional Helpers

Kind: `functions`

### `map`

- Syntax: `map(arr, fn)`
- Summary: Maps array values.

### `filter`

- Syntax: `filter(arr, fn)`
- Summary: Filters array by predicate.

### `reduce`

- Syntax: `reduce(arr, fn, initial)`
- Summary: Reduces array.

### `find`

- Syntax: `find(arr, fn)`
- Summary: First element matching predicate.

### `some`

- Syntax: `some(arr, fn)`
- Summary: True if any match.

### `every`

- Syntax: `every(arr, fn)`
- Summary: True if all match.

### `count`

- Syntax: `count(arr, fn)`
- Summary: Count matching predicate.

### `pluck`

- Syntax: `pluck(arr, key)`
- Summary: Extract field from each object.

### `group_by`

- Syntax: `group_by(arr, keyOrFn)`
- Summary: Groups by key/selector.

### `sum`

- Syntax: `sum(arr)`
- Summary: Numeric sum.

### `min`

- Syntax: `min(a, b[, ...])`
- Summary: Minimum value.

### `max`

- Syntax: `max(a, b[, ...])`
- Summary: Maximum value.

### `clamp`

- Syntax: `clamp(value, lo, hi)`
- Summary: Clamps value to range.

## Math Functions

Kind: `functions`

### `rand`

- Syntax: `rand() | rand(max) | rand(min, max)`
- Summary: Random integer helper.

### `abs`

- Syntax: `abs(n)`
- Summary: Absolute value.

### `ceil`

- Syntax: `ceil(n)`
- Summary: Round up.

### `floor`

- Syntax: `floor(n)`
- Summary: Round down.

### `round`

- Syntax: `round(n)`
- Summary: Round nearest.

## Encoding and Hashing

Kind: `functions`

### `base64_encode`

- Syntax: `base64_encode(text)`
- Summary: Base64 encode.

### `base64_decode`

- Syntax: `base64_decode(text)`
- Summary: Base64 decode.

### `url_encode`

- Syntax: `url_encode(text)`
- Summary: URL encode.

### `url_decode`

- Syntax: `url_decode(text)`
- Summary: URL decode.

### `hash`

- Syntax: `hash(value[, algorithm])`
- Summary: Hash helper (sha256 default).

### `hmac_hash`

- Syntax: `hmac_hash(value, secret[, algorithm])`
- Summary: HMAC helper.

### `hash_password`

- Syntax: `hash_password(password[, opts])`
- Summary: Password hash.

### `verify_password`

- Syntax: `verify_password(password, hash)`
- Summary: Password hash verification.

### `uuid`

- Syntax: `uuid()`
- Summary: UUID helper.

### `cuid2`

- Syntax: `cuid2()`
- Summary: CUID2 helper.

## Date and Time

Kind: `functions`

### `date`

- Syntax: `date([value])`
- Summary: Date object/value constructor.

### `date_format`

- Syntax: `date_format(value, format)`
- Summary: Date formatting.

### `date_parse`

- Syntax: `date_parse(text[, format])`
- Summary: Date parsing.

### `strtotime`

- Syntax: `strtotime(text)`
- Summary: Parse relative/time text to timestamp.

## Validation

Kind: `functions`

### `validate`

- Syntax: `validate(value, schema)`
- Summary: Schema-based validation.

### `is_email`

- Syntax: `is_email(text)`
- Summary: Email format check.

### `is_url`

- Syntax: `is_url(text)`
- Summary: URL format check.

### `is_uuid`

- Syntax: `is_uuid(text)`
- Summary: UUID format check.

### `is_numeric`

- Syntax: `is_numeric(value)`
- Summary: Numeric check.

## json Namespace

Kind: `namespace`

### `json.parse`

- Syntax: `json.parse(text)`
- Summary: Parses JSON text.

### `json.stringify`

- Syntax: `json.stringify(value)`
- Summary: Serializes JSON text.

## file Namespace

Kind: `namespace`

### `file.open`

- Syntax: `file.open(path[, mode])`
- Summary: Opens file handle.

### `file.read`

- Syntax: `file.read(path)`
- Summary: Reads file.

### `file.write`

- Syntax: `file.write(path, data)`
- Summary: Writes file.

### `file.append`

- Syntax: `file.append(path, data)`
- Summary: Appends file.

### `file.read_json`

- Syntax: `file.read_json(path)`
- Summary: Reads JSON file.

### `file.write_json`

- Syntax: `file.write_json(path, value)`
- Summary: Writes JSON file.

### `file.exists`

- Syntax: `file.exists(path)`
- Summary: File existence check.

### `file.delete`

- Syntax: `file.delete(path)`
- Summary: Deletes file.

### `file.list`

- Syntax: `file.list(path)`
- Summary: Lists directory entries.

### `file.mkdir`

- Syntax: `file.mkdir(path)`
- Summary: Creates directory.

### `file.chmod`

- Syntax: `file.chmod(path, mode)`
- Summary: Changes file mode.

## File Handle Methods

Kind: `methods`

### `read`

- Syntax: `fh.read()`
- Summary: Reads from opened file handle.

### `write`

- Syntax: `fh.write(data)`
- Summary: Writes through file handle.

### `append`

- Syntax: `fh.append(data)`
- Summary: Appends through file handle.

### `lines`

- Syntax: `fh.lines([limit])`
- Summary: Reads lines.

### `json`

- Syntax: `fh.json()`
- Summary: Reads JSON from file handle.

### `write_json`

- Syntax: `fh.write_json(value)`
- Summary: Writes JSON to file handle.

### `exists`

- Syntax: `fh.exists()`
- Summary: Checks handle path existence.

### `size`

- Syntax: `fh.size()`
- Summary: Reads file size.

### `delete`

- Syntax: `fh.delete()`
- Summary: Deletes file at handle path.

### `path`

- Syntax: `fh.path()`
- Summary: Returns handle path.

## DB and JWT Namespaces

Kind: `namespace`

### `db.open`

- Syntax: `db.open(dsnOrConfig)`
- Summary: Opens database handle.

### `jwt.sign`

- Syntax: `jwt.sign(payload, secret[, opts])`
- Summary: JWT signing.

### `jwt.verify`

- Syntax: `jwt.verify(token, secret[, opts])`
- Summary: JWT verification.

## DB Handle Methods

Kind: `methods`

### `exec`

- Syntax: `dbh.exec(query[, args])`
- Summary: Executes write/query statement.

### `query`

- Syntax: `dbh.query(query[, args])`
- Summary: Returns row collection.

### `query_one`

- Syntax: `dbh.query_one(query[, args])`
- Summary: Returns first row.

### `query_value`

- Syntax: `dbh.query_value(query[, args])`
- Summary: Returns first scalar value.

### `insert`

- Syntax: `dbh.insert(collection, doc)`
- Summary: Mongo-like insert.

### `insert_many`

- Syntax: `dbh.insert_many(collection, docs)`
- Summary: Mongo-like bulk insert.

### `find`

- Syntax: `dbh.find(collection, filter)`
- Summary: Mongo-like find.

### `find_one`

- Syntax: `dbh.find_one(collection, filter)`
- Summary: Mongo-like find one.

### `update`

- Syntax: `dbh.update(collection, filter, patch)`
- Summary: Mongo-like update.

### `delete`

- Syntax: `dbh.delete(collection, filter)`
- Summary: Mongo-like delete.

### `count`

- Syntax: `dbh.count(collection[, filter])`
- Summary: Mongo-like count.

### `close`

- Syntax: `dbh.close()`
- Summary: Closes DB handle.

## store Namespace

Kind: `namespace`

### `store.get`

- Syntax: `store.get(key[, fallback])`
- Summary: Gets store value.

### `store.set`

- Syntax: `store.set(key, value[, ttlSeconds])`
- Summary: Sets store value.

### `store.delete`

- Syntax: `store.delete(key)`
- Summary: Deletes key.

### `store.has`

- Syntax: `store.has(key)`
- Summary: Existence check.

### `store.all`

- Syntax: `store.all()`
- Summary: Returns full key-value snapshot.

### `store.incr`

- Syntax: `store.incr(key[, by[, ttlSeconds]])`
- Summary: Atomically increments numeric key.

### `store.sync`

- Syntax: `store.sync([path])`
- Summary: Flushes store state.

## SSE Namespaces

Kind: `namespace`

### `stream.send`

- Syntax: `stream.send([event,] data)`
- Summary: Sends event on current SSE stream.
- Availability: Inside SSE route.

### `stream.join`

- Syntax: `stream.join(channel)`
- Summary: Joins channel.
- Availability: Inside SSE route.

### `stream.leave`

- Syntax: `stream.leave(channel)`
- Summary: Leaves channel.
- Availability: Inside SSE route.

### `stream.set`

- Syntax: `stream.set(key, value)`
- Summary: Sets stream metadata.
- Availability: Inside SSE route.

### `stream.get`

- Syntax: `stream.get(key)`
- Summary: Gets stream metadata.
- Availability: Inside SSE route.

### `stream.close`

- Syntax: `stream.close()`
- Summary: Closes current stream.
- Availability: Inside SSE route.

### `stream.channels`

- Syntax: `stream.channels()`
- Summary: Lists current stream channels.
- Availability: Inside SSE route.

### `sse.find`

- Syntax: `sse.find(streamId)`
- Summary: Finds stream handle by ID.

### `sse.find_by`

- Syntax: `sse.find_by(key, value)`
- Summary: Finds streams by metadata field.

### `sse.channel`

- Syntax: `sse.channel(name)`
- Summary: Returns channel handle.

### `sse.broadcast`

- Syntax: `sse.broadcast(data)`
- Summary: Broadcasts all streams.

### `sse.count`

- Syntax: `sse.count()`
- Summary: Connected stream count.

### `sse.channels`

- Syntax: `sse.channels()`
- Summary: Known channel list.

## SSE Handle Methods

Kind: `methods`

### `send`

- Syntax: `handle.send([event,] data)`
- Summary: Sends event to a stream handle.

### `set`

- Syntax: `handle.set(key, value)`
- Summary: Sets handle metadata.

### `get`

- Syntax: `handle.get(key)`
- Summary: Gets handle metadata.

### `join`

- Syntax: `handle.join(channel)`
- Summary: Joins channel.

### `leave`

- Syntax: `handle.leave(channel)`
- Summary: Leaves channel.

### `close`

- Syntax: `handle.close()`
- Summary: Closes handle stream.

### `channels`

- Syntax: `handle.channels()`
- Summary: Lists handle channels.

### `streams`

- Syntax: `channelHandle.streams()`
- Summary: Lists streams in channel.

### `count`

- Syntax: `channelHandle.count()`
- Summary: Counts streams in channel.

