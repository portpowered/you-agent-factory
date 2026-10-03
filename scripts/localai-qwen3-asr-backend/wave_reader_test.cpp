#include "wave_reader.h"
#include <cstdlib>
#include <cstdio>
#include <windows.h>
#include <winioctl.h>

static void put(std::ofstream& file, uint64_t value, size_t bytes) {
    for (size_t i=0;i<bytes;++i) file.put(static_cast<char>(value>>(i*8)));
}
static void fixture(const std::filesystem::path& path, uint32_t frames, bool rf64=false, bool floating=false) {
    std::ofstream f(path,std::ios::binary);
    const auto bytes=frames*(floating?4:2), riff_size=4+12+24+8+bytes+(rf64?36:0);
    f.write(rf64?"RF64":"RIFF",4);put(f,rf64?UINT32_MAX:riff_size,4);f.write("WAVE",4);
    if(rf64){f.write("ds64",4);put(f,28,4);put(f,riff_size,8);put(f,bytes,8);put(f,frames,8);put(f,0,4);}
    // Odd metadata chunks have one padding byte before the next header.
    f.write("JUNK",4);put(f,3,4);f.write("abc",3);f.put(0);
    f.write("fmt ",4);put(f,16,4);put(f,floating?3:1,2);put(f,1,2);put(f,16000,4);
    put(f,16000*(floating?4:2),4);put(f,floating?4:2,2);put(f,floating?32:16,2);
    f.write("data",4);put(f,rf64?UINT32_MAX:bytes,4);
    for(uint32_t i=0;i<frames;++i) {
        if(floating){float sample=.25f;uint32_t bits;std::memcpy(&bits,&sample,4);put(f,bits,4);}
        else put(f,i%32768,2);
    }
}
static void sparse_rf64(const std::filesystem::path& path) {
    fixture(path,0,true);
    HANDLE handle=CreateFileW(path.c_str(),GENERIC_WRITE,FILE_SHARE_READ,nullptr,OPEN_EXISTING,FILE_ATTRIBUTE_NORMAL,nullptr);
    if(handle==INVALID_HANDLE_VALUE)throw std::runtime_error("Could not open sparse RF64 fixture");
    try {
        DWORD returned;
        if(!DeviceIoControl(handle,FSCTL_SET_SPARSE,nullptr,0,nullptr,0,&returned,nullptr))throw std::runtime_error("NTFS sparse files are required for RF64 regression");
        constexpr uint64_t bytes=uint64_t(UINT32_MAX)+17, file_bytes=92+bytes;
        LARGE_INTEGER position;position.QuadPart=file_bytes;
        if(!SetFilePointerEx(handle,position,nullptr,FILE_BEGIN)||!SetEndOfFile(handle))throw std::runtime_error("Could not size sparse RF64 fixture");
        auto write_at=[&](uint64_t offset,uint64_t value,DWORD count){
            unsigned char data[8];for(DWORD i=0;i<count;++i)data[i]=static_cast<unsigned char>(value>>(8*i));
            position.QuadPart=offset;DWORD written;
            if(!SetFilePointerEx(handle,position,nullptr,FILE_BEGIN)||!WriteFile(handle,data,count,&written,nullptr)||written!=count)throw std::runtime_error("Could not write sparse RF64 evidence");
        };
        write_at(20,file_bytes-8,8);write_at(28,bytes,8);write_at(36,bytes/2,8);write_at(file_bytes-2,0x4000,2);
        CloseHandle(handle);handle=INVALID_HANDLE_VALUE;
        WaveReader reader(path.string());auto tail=reader.window(reader.frames-2,2);
        if(reader.frames!=bytes/2 || tail.size()!=2 || tail[0]!=0 || tail[1]!=.5f)throw std::runtime_error("RF64 seek lost audio beyond 4 GiB");
    }catch(...){if(handle!=INVALID_HANDLE_VALUE)CloseHandle(handle);throw;}
}
int main() {
    const auto root=std::filesystem::temp_directory_path()/"qwen-asr-window-reader-test";
    std::filesystem::create_directories(root);
    try {
        { const auto pcm=root/"pcm.wav";fixture(pcm,50*16000);
        WaveReader reader(pcm.string());auto first=reader.window(0,30*16000),second=reader.window(28*16000,30*16000);
        if(reader.frames!=50*16000 || first.size()!=30*16000 || second.size()!=22*16000 || first[28*16000]!=second[0]){std::fprintf(stderr,"WAV reader regression failed at line %d\n",__LINE__);return EXIT_FAILURE;}
        if(first[1]!=1/32768.0f || second.back()!=float((50*16000-1)%32768)/32768){std::fprintf(stderr,"WAV reader regression failed at line %d\n",__LINE__);return EXIT_FAILURE;}
        const auto rf64=root/"rf64.wav";fixture(rf64,100,true);WaveReader extended(rf64.string());if(extended.frames!=100||extended.window(90,30).size()!=10){std::fprintf(stderr,"WAV reader regression failed at line %d\n",__LINE__);return EXIT_FAILURE;}
        sparse_rf64(root/"large-rf64.wav");
        const auto fp=root/"float.wav";fixture(fp,100,false,true);WaveReader floating(fp.string());if(floating.window(0,10)[0]!=.25f){std::fprintf(stderr,"WAV reader regression failed at line %d\n",__LINE__);return EXIT_FAILURE;}
        const auto truncated=root/"truncated.wav";fixture(truncated,100);std::filesystem::resize_file(truncated,90);
        bool rejected=false;try{WaveReader invalid(truncated.string());}catch(const std::runtime_error&){rejected=true;}if(!rejected){std::fprintf(stderr,"WAV reader regression failed at line %d\n",__LINE__);return EXIT_FAILURE;}
        const auto malformed=root/"malformed.wav";std::ofstream bad(malformed,std::ios::binary);bad.write("not WAV",7);bad.close();
        rejected=false;try{WaveReader invalid(malformed.string());}catch(const std::runtime_error&){rejected=true;}if(!rejected){std::fprintf(stderr,"WAV reader regression failed at line %d\n",__LINE__);return EXIT_FAILURE;}
        } std::filesystem::remove_all(root);return EXIT_SUCCESS;
    }catch(const std::exception& e){std::fprintf(stderr,"reader exception: %s\n",e.what());std::filesystem::remove_all(root);return EXIT_FAILURE;}
}
