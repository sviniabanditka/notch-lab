# Writing rules

All documents in this repository use ASD-STE100 Simplified Technical English. These rules apply.

## Sentences

- Write one instruction per sentence.
- Use no more than 20 words in a procedural sentence.
- Use no more than 25 words in a descriptive sentence.
- Use the active voice. Write "The Daemon starts the Session", not "The Session is started".
- Use the present tense for facts. Use the imperative for instructions.
- Start an instruction with the verb.
- Do not use "should", "would", "could", "may" for requirements. Write "must" for a requirement. Write "can" for an ability.
- Do not use "it", "this", "that" alone when the reader can read it two ways. Repeat the noun.

## Words

- Use the words in `CONTEXT.md`. Do not use the words marked _Avoid_.
- Use one word for one meaning. Use one meaning for one word.
- Do not use slang, idioms or metaphors.
- Do not use a noun cluster of more than three nouns. Write "the limit of the Scrollback", not "Daemon Session Scrollback line limit".
- Write numbers as digits: "3 seconds", not "three seconds".
- Write units after a space: "120 pt", "500 ms", "64 KB".
- Spell a key combination with "+": "Cmd+Shift+D".

## Requirements

- Number requirements inside each document as `R-<module>-<number>`, for example `R-TERM-012`.
- A requirement has one testable statement.
- Write the acceptance test under the requirement where the test is not obvious.
- Mark a requirement that is not in v1 with `[later]`.

## Lists and tables

- Use a list for steps and for parallel items.
- Put one step in one list item.
- Use a table when three or more items have the same attributes.
