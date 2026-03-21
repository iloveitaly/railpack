import platform
import sys
import sysconfig

# Py_GIL_DISABLED == 1 means the binary was built without the GIL (free-threaded build)
is_freethreaded = sysconfig.get_config_var("Py_GIL_DISABLED") == 1

print(f"Python {sys.version.split()[0]}")
print(f"Free-threaded build: {is_freethreaded}")
print(f"Architecture: {platform.machine()}")
