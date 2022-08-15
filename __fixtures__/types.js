// `import type`/`import typeof` imports will be ignored
import type Foo from 'EXCLUDED';
import typeof Foo from 'EXCLUDED';
// however, if `type` is used within the brackets, it'll be included (even if it's the only thing listed)
import {type Foo, bar} from 'INCLUDED-1';
import {type Baz} from 'INCLUDED-2';
