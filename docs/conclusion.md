# Conclusion
While the project did not reach its ultimate conclusion, it has shown that it makes sense to add permission-based linearity to Go.

It also shows that the implementation of permissions with a few primitive rules can be used to construct static analysers for permissions in programs, even though the abstract interpreter is not fully capable yet.

Further work is needed to construct a full analyser: Support for complete packages needs to be added, support for importing other packages, and importantly, also support for conversions between non-interface and interface values.

The disciplined unit testing led to a high quality code base for the basic permission operations, which made it easy to add new features without breaking a lot of stuff. It also leads me to believe that any future work can likely reuse them, and just needs to extend them with named permissions and packages, and extend or rewrite the interpreter to support named permissions.
