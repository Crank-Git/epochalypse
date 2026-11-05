# Name

_unpack_ - unpack the initial query

# Description

The _unpack_ handler unpacks the full message. It is the automatically set to be the first handler to run.
The handler also registers DNS context functions to make atomdns automatically (E)DNS compliant, see RFC 8906.
It:

- Sets the recursion desired bit to reflect the query.
- Sets recursion available to false.
- Sets z bit to false.

You can not use it in the configuration, nor alter its behavior.

# Syntax

For documentation purposes the syntax is shown here.

```txt
unpack
```

# See Also

RFC 8906.
