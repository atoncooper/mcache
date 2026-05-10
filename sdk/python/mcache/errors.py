class McacheError(Exception):
    """Base exception for all mcache errors."""
    pass


class ConnectionError(McacheError):
    """Failed to connect or connection lost."""
    pass


class ReadTimeout(McacheError):
    """Read operation timed out."""
    pass


class KeyNotFoundError(McacheError):
    """Key does not exist."""
    pass


class ServerError(McacheError):
    """Server returned an error."""
    pass


class ProtocolError(McacheError):
    """Invalid or malformed response from server."""
    pass


class InvalidCommandError(McacheError):
    """Unknown or invalid command."""
    pass


class PoolExhaustedError(McacheError):
    """No available connections in the pool."""
    pass
