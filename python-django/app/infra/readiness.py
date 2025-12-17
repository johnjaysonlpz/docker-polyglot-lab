from threading import Lock

class ReadinessState:
    def __init__(self) -> None:
        self._lock = Lock()
        self._accepting_traffic = True

    def set_accepting(self, accepting: bool) -> None:
        with self._lock:
            self._accepting_traffic = accepting

    def is_accepting(self) -> bool:
        with self._lock:
            return self._accepting_traffic

state = ReadinessState()
