FROM golang:1.24

# Install required packages
RUN apt-get update && apt-get install -y \
    gcc-aarch64-linux-gnu \
    g++-aarch64-linux-gnu \
    build-essential \
    cmake \
    git \
    libgtk2.0-dev \
    pkg-config \
    libavcodec-dev \
    libavformat-dev \
    libswscale-dev \
    libtbb2 \
    libtbb-dev \
    libjpeg-dev \
    libpng-dev \
    libtiff-dev

# Set up OpenCV
WORKDIR /tmp
RUN git clone --depth 1 --branch 4.8.0 https://github.com/opencv/opencv.git && \
    git clone --depth 1 --branch 4.8.0 https://github.com/opencv/opencv_contrib.git

# Build OpenCV for ARM64
WORKDIR /tmp/opencv/build
RUN cmake -D CMAKE_BUILD_TYPE=RELEASE \
    -D CMAKE_INSTALL_PREFIX=/usr/local/opencv-arm64 \
    -D OPENCV_EXTRA_MODULES_PATH=/tmp/opencv_contrib/modules \
    -D CMAKE_TOOLCHAIN_FILE=../platforms/linux/aarch64-gnu.toolchain.cmake \
    -D ENABLE_NEON=ON \
    -D WITH_OPENGL=OFF \
    -D BUILD_TESTS=OFF \
    -D BUILD_PERF_TESTS=OFF \
    -D BUILD_EXAMPLES=OFF \
    -D BUILD_opencv_python2=OFF \
    -D BUILD_opencv_python3=OFF \
    -D OPENCV_GENERATE_PKGCONFIG=ON .. && \
    make -j$(nproc) && \
    make install

# Set environment variables
ENV PKG_CONFIG_PATH=/usr/local/opencv-arm64/lib/pkgconfig:$PKG_CONFIG_PATH
ENV LD_LIBRARY_PATH=/usr/local/opencv-arm64/lib:$LD_LIBRARY_PATH
ENV CGO_CPPFLAGS="-I/usr/local/opencv-arm64/include"
ENV CGO_LDFLAGS="-L/usr/local/opencv-arm64/lib -lopencv_core -lopencv_face -lopencv_videoio -lopencv_imgproc -lopencv_highgui -lopencv_imgcodecs -lopencv_objdetect -lopencv_features2d -lopencv_video -lopencv_dnn -lopencv_calib3d"

# Copy your code
WORKDIR /app
COPY . .

# Build the application
CMD ["go", "run", "tasks.go", "build"]
